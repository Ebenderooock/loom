# ADR-0015: qBittorrent download client kind (Phase 3b)

- Status: Accepted
- Date: 2025-02-13
- Deciders: @loom-maintainers

## Context

ADR-0014 (Phase 3a) established `internal/downloads/` as the registry
+ service + repository layer for download clients. Phase 3b is the
first real driver to land on top of that foundation: qBittorrent. We
need to commit to:

- the qBittorrent API generation we target,
- how authentication and session management work,
- how the rich qBittorrent state vocabulary collapses onto Loom's
  small `ItemStatus` enum,
- what the retry semantics are when the session cookie expires, and
- which qBittorrent features we are explicitly *not* implementing yet.

## Decision

### API version

Loom targets the **v2 Web API** (`/api/v2/...`), introduced in
qBittorrent 4.1.0 and unchanged in semantics through 5.x. Anything
older than 4.1 is unsupported. We make no use of v5-only endpoints,
so the same client speaks to both major versions.

### Authentication

We use cookie-based auth via `POST /api/v2/auth/login` with
form-encoded `username` / `password` and persist the resulting `SID`
cookie in an `http.CookieJar` shared with the per-client
`*http.Client`. We set `Referer` and `Origin` headers to the
configured base URL on every request so that qBittorrent's CSRF /
host-validation logic accepts the call when the WebUI is fronted by
a reverse proxy on a subpath.

### Re-login on 403

A single 403 response on any authenticated endpoint triggers exactly
one transparent re-login + retry. The `loginMu` mutex serialises
re-logins so a burst of expired-cookie 403s does not fan out into N
parallel `auth/login` requests. If the retry also returns 403 we
surface `ErrAuthFailed`. Any non-403, non-2xx status surfaces as
`ErrServer` wrapping the path, status code, and a truncated body.

### State mapping

qBittorrent reports a per-torrent state string from a vocabulary of
~18 values. We collapse them onto `ItemStatus` in a single named
function (`mapState`) so the table can be audited at a glance:

| qBittorrent state                                            | Loom status   |
| ------------------------------------------------------------ | ------------- |
| `downloading`, `forcedDL`, `metaDL`, `checkingDL`, `stalledDL`, `allocating`, `moving`, `checkingResumeData` | `downloading` |
| `queuedDL`, `queuedUP`                                       | `queued`      |
| `uploading`, `forcedUP`, `stalledUP`, `checkingUP`           | `seeding`     |
| `pausedDL`                                                   | `paused`      |
| `pausedUP`                                                   | `completed`   |
| `error`, `missingFiles`                                      | `failed`      |
| anything else                                                | `unknown`     |

The `pausedUP` → `completed` mapping is the only non-obvious entry: a
paused-while-seeding torrent is finished work from the operator's
perspective, even though qBittorrent itself still marks it "paused".
Treating it as `completed` lets dashboards and the upcoming queue
logic differentiate "owes bytes" from "has bytes".

### Item id is the v1 infohash

qBittorrent does not echo the infohash on `/torrents/add` (the
endpoint replies `Ok.` regardless of input). Loom computes the v1
infohash client-side:

- **Magnet** — extract from the `xt=urn:btih:` parameter.
- **Raw `.torrent` bytes** — bencode-parse the metainfo just enough
  to find the byte range of the `info` dict and SHA-1 it. The
  parser is intentionally minimal (~30 lines).
- **`.torrent` URL** — left empty; the row will surface on the next
  `Status` poll once qBittorrent has fetched and parsed the file.

### Transport composition

The factory consumes `downloads.TransportForDefinition`, exactly the
way indexer kinds consume `indexers.TransportForDefinition`. Per-client
proxy and throttle layering are therefore identical to indexers and
will pick up future transport policies without code change here.

### Deferred features

The following are explicitly out of scope for Phase 3b:

- **Torrent priority / queue position** — `/torrents/topPrio`,
  `/bottomPrio`, `/increasePrio`, `/decreasePrio`. The downloads
  abstraction does not yet have a priority concept; revisit when
  the queue subsystem lands.
- **File selection** — `/torrents/files`, `/torrents/filePrio`.
  Same reason: no abstraction in `DownloadClient` to surface it
  through.
- **Tracker management** — `/torrents/trackers`, `/addTrackers`.
- **Provisioning categories from Loom** — only round-tripping
  (`Categories()`) is supported; `Add` honours
  `req.Category` and qBittorrent 4.4+ will lazily create unknown
  categories.

These are tracked as future work; the API surface is
forward-compatible.

## Consequences

### Positive

- A real driver lands without changes to the Phase 3a foundation —
  the abstraction held.
- The state-mapping table lives in one auditable function.
- 403 → re-login is centralised in `Client.do`, so no per-endpoint
  helper has to remember to handle session expiry.

### Negative / trade-offs

- We compute infohashes client-side instead of asking qBittorrent.
  The bencode parser is small but is one more thing to maintain. The
  alternative — round-tripping `Status` after `Add` to discover the
  hash — is racy and slower.
- `pausedUP` → `completed` is opinionated and may surprise operators
  who expect a 1:1 mapping. We trade strict fidelity for a
  cleaner mental model.

### Neutral

- We elected not to store the qBittorrent server version in the
  health row even though `Test` already fetches it. The next phase
  can add it without a wire break.

## Alternatives considered

- **Use the v1 API** — rejected. Deprecated; 4.1+ servers do not
  guarantee v1 endpoints stay around.
- **Per-call `auth/login`** — rejected. Each call would burn a TCP
  round-trip and accelerate qBittorrent's rate-limiting on bad
  passwords (which is per-IP). Cookie reuse + transparent re-login
  is strictly better.
- **Skip infohash computation** — rejected. Without an item id
  callers cannot pass specific torrents back into Pause/Resume/Remove
  before the next `Status` poll.
