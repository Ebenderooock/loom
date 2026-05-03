# Cardigann YAML indexer kind

The `cardigann` indexer kind lets Loom drive trackers that publish a
[Cardigann](https://github.com/cardigann/cardigann)-style YAML
definition. The YAML describes the tracker's login flow and search
HTML so the same engine code can talk to dozens of sites without one
hand-written client per tracker. This is how Jackett and Prowlarr
support their long tail of trackers; Loom uses the same idea while
keeping the trust boundary tight.

## Trust model

- **Definitions live on disk only.** The Loom API never accepts an
  inline YAML body; an indexer row references an existing definition
  by its filename (sans extension). Operators copy or fetch
  definitions out-of-band, the same way they distribute server
  certs.
- **Definitions are not code.** The engine evaluates a fixed set of
  CSS / XPath selectors and a small filter chain. Unknown filter
  names are passed through unchanged rather than executed; there is
  no template or scripting evaluation outside Go's `text/template`
  with a closed set of context fields.
- **Credentials never leave the indexer config.** Username, password,
  passkey, and any free-form `credentials` map are persisted in the
  indexer row's JSON config. They are templated into login form
  inputs and search inputs (`{{ .Config.username }}`) but are never
  echoed back to the API caller in plaintext.

## Layout

By default the loader scans `<data_dir>/definitions/cardigann/`,
recursively, picking up any `*.yml` or `*.yaml` file. Each file's
basename (without extension) is the **definition ID** that an indexer
row points at — so renaming `tracker-private.yml` to
`tracker-public.yml` flips the indexer over to the second variant
without touching the JSON config.

Set `indexers.cardigann.definitions_dir` in `loom.yaml` (or the
matching env var) to override the location. A missing directory is
not an error: the kind reports zero definitions, and any indexer
rows pointing at a missing definition surface a clear error at
hydrate time.

## Indexer config schema

The `Indexer.config` blob looks like:

```json
{
  "definition_id": "exampletracker",
  "username": "alice",
  "password": "hunter2",
  "passkey": "0123456789abcdef",
  "category_overrides": {"42": 2040},
  "credentials": {"two_factor": "123456"},
  "user_agent": "Loom/0.1",
  "timeout": "30s"
}
```

| Field | Required | Notes |
|---|---|---|
| `definition_id` | yes | Filename of the on-disk YAML, sans extension. |
| `username` / `password` | login-flow | Templated into form-login inputs. |
| `passkey` | optional | Available as `{{ .Config.passkey }}` for sites that embed a per-user key in download URLs. |
| `cookie` | optional | `name=value; name2=value2` — pre-baked session, used in lieu of form login. |
| `credentials` | optional | Free-form map exposed as `{{ .Config.<key> }}`. |
| `category_overrides` | optional | Site-id → Newznab-id, beats the YAML's `categorymappings`. |
| `user_agent` | optional | Override the outbound User-Agent. Defaults to `Loom/0.1`. |
| `timeout` | optional | Go duration. Default `30s`. |

The legacy alias `definition` is also accepted in place of
`definition_id`.

## Supported / deferred YAML surface

Cardigann's full schema is large; Phase 2b implements the parts
needed to drive most public and private tracker definitions in the
wild. The matrix below is the contract — anything outside it is
either rejected at load time (with a clear error) or silently
ignored.

| Area | Supported | Deferred |
|---|---|---|
| Login modes | `form`, `post`, `cookie` | `get`, captcha-challenge, OAuth |
| Selector dispatch | CSS (default), XPath when the selector starts with `/` or `(` | xpath/css explicit prefixes |
| Filters | `trim`, `lowercase`, `uppercase`, `replace`, `regexp`, `querystring`, `prepend`, `append`, `split`, `join` | `dateparse`, `urlencode`, `htmldecode`, others (pass-through, value preserved) |
| Search inputs | `{{ .Keywords }}`, `{{ .Categories }}`, `{{ .IMDBID }}`, `{{ .Season }}`, `{{ .Episode }}`, `{{ .Config.* }}`, `$raw` for pre-encoded fragments | Conditional templates (`if`/`with`) |
| Result fields | `title`, `download`, `link`, `details`, `comments`, `size`, `date`, `seeders`, `peers`, `leechers`, `category`, `magnet`, `infohash`, `quality` | Per-row details-page fetch |
| Categories | `categorymappings` (id → Newznab name) and case-insensitive name lookup | Hierarchical inheritance |
| Ratio enforcement | — | Tracked for a future phase |
| Multi-link failover | — | First link is authoritative |
| Definition repo sync | — | Operators distribute YAML by hand |

## Curl walkthrough

Drop `definitions/cardigann/exampletracker.yml` into your data dir,
then create the indexer:

```bash
curl -sS -X POST "http://127.0.0.1:8989/api/v1/indexers/" \
  -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "kind": "cardigann",
    "name": "ExampleTracker",
    "config": {
      "definition_id": "exampletracker",
      "username": "alice",
      "password": "hunter2"
    }
  }'
```

Test the credentials:

```bash
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://127.0.0.1:8989/api/v1/indexers/<id>/test
```

Search:

```bash
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"query":"ubuntu","categories":[5040]}' \
  http://127.0.0.1:8989/api/v1/indexers/search
```

## Debugging

- **`definition "X" not found under "<dir>"`** — the YAML file is
  missing or its basename does not match `definition_id`. Files must
  end in `.yml` or `.yaml`.
- **`login rejected: ...`** — the YAML's `login.error[]` selectors
  matched. Re-check credentials, or run a single search with
  `LOOM_LOG_LEVEL=debug` to see the response body.
- **`login test selector did not match`** — the form post returned
  200 but the verification selector (e.g. a "Logout" link) is
  absent. Common when the tracker silently rate-limits a fresh
  session.
- **Empty results, no error** — the row selector did not match.
  Check the live HTML in a browser; the YAML may be stale and need a
  selector update.

## See also

- [ADR-0012](adr/0012-cardigann-yaml-definition-loader.md) — the
  decision record covering library choice and trust model.
- [indexers.md](indexers.md) — the cross-kind core (registry,
  health, fan-out search).
- Upstream Cardigann schema:
  <https://github.com/cardigann/cardigann/blob/master/DEFINITIONS.md>.
