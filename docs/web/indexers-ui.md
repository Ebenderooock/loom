# Indexers & Proxies UI

The Loom web UI ships with two management pages — **Indexers** and
**Proxies** — that let you configure the search backends Loom queries and the
optional proxies it routes those requests through. This page walks through
the day-to-day flows.

For the underlying REST API, see [`api/openapi/loom.yaml`](../../api/openapi/loom.yaml)
and [ADR-0009 — Indexers & Proxies](../adr/0009-indexer-proxies.md). For the
UI design rationale see [ADR-0010](../adr/0010-indexers-and-proxies-ui.md).

## Navigating to the pages

Both pages live in the left navigation:

- **Indexers** (radio icon) — `/indexers`
- **Proxies** (network icon) — `/proxies`

## Adding an indexer

1. Open **Indexers → Add indexer**.
2. Fill in the form:
   - **Kind** — `torznab` or `newznab`. Pick the one your provider documents.
   - **Name** — display name (must be unique).
   - **URL** — base URL of the indexer endpoint (must be `http`/`https`).
   - **API key** — optional; required by most providers.
   - **Priority** — integer, lower wins on result ranking ties. Default `25`.
   - **Timeout (ms)** — per-request timeout. Default `15000`.
   - **Categories** — comma-separated Newznab/Torznab category IDs (e.g.
     `2000,5000`). Optional; if empty Loom queries the indexer's defaults.
   - **Tags** — comma-separated free-form labels for grouping/filtering.
   - **Proxy** — optional; pick a configured proxy from the dropdown. Leave
     as **None** for a direct connection.
   - **Enabled** — toggle to disable without deleting.
3. **Save**.

The list refreshes and the new indexer appears with a health badge. The
backend's health worker probes new indexers within a few seconds.

## Editing an indexer

Use the **⋯** menu on a row → **Edit**. The same form opens populated with
current values. Only changed fields are sent (`PATCH` semantics):

- Leaving a field unchanged means it is omitted from the request and the
  server keeps its current value.
- The **Proxy** dropdown is special — see *Detaching a proxy* below.

## Attaching and detaching a proxy

The **Proxy** dropdown on the indexer form has three meaningful states:

| Form state | What Loom sends | Effect |
|---|---|---|
| Unchanged | (field omitted) | Existing proxy kept as-is. |
| Set to **None** when a proxy was previously attached | `proxy_id: null` | Proxy detached; indexer goes direct. |
| Set to a proxy | `proxy_id: "<uuid>"` | New proxy attached (or swapped). |

This null-vs-omit distinction matches the OpenAPI contract and is what lets
the UI safely round-trip partial edits.

You can also delete a proxy from the **Proxies** page; if any indexer still
references it the API returns `409 proxy_in_use` and the UI surfaces the
error inline so you can detach the indexer first.

## Manual search

Each indexer row exposes a **Search** action. It opens a dialog backed by
the API's search endpoint, scoped to that single indexer (the request is a
fan-out search with `indexer_ids: ["<this-id>"]`). Enter a query, hit
**Search**, and the results table shows title, size, seeders/peers (for
torrents), publish date, indexer, and a download link.

This is the same path users will hit via the search/discovery UI later
(Phase 5+). It's exposed here so you can sanity-check an indexer's
connectivity right after adding it.

## Health badges

Every indexer shows a coloured health badge derived from `health.status`
and `health.last_checked_at`:

| Badge | Meaning |
|---|---|
| **Unknown** (gray) | Never probed yet (e.g. just created). |
| **OK** (green) | Last probe succeeded. |
| **Stale** (amber) | Last probe was OK but is older than 24 h — likely the health worker has stalled. |
| **Degraded** (amber) | Probe succeeded with warnings (slow, partial). |
| **Failed** (red) | Last probe failed. Hover the badge for the latency/error detail in the table cell beside it. |

Health badges update reactively — the page polls the indexers list via
TanStack Query, so once the backend records a new probe the UI updates on
the next refetch (default 30 s).

## Proxies page

The Proxies page lists all configured proxies with kind, masked
URL/address, and tag count. Add via **Add proxy**. Supported kinds:

- **HTTP / HTTPS** — `url` (required), optional `username` / `password`.
- **SOCKS5** — `address` (host:port), optional `username` / `password`.
- **FlareSolverr** — `url` (required), optional `max_timeout_ms`,
  `session_mode` (`per_indexer` | `shared`).

Credentials in URLs are masked in the table (e.g. `https://***:***@host`).
The full credentials live only in the form and on the server.

## Accessibility

- All form fields use `<Label>` with explicit `htmlFor` and inputs have
  `aria-invalid` + `aria-describedby` when validation fails.
- Error messages render as `role="alert"` so screen readers announce them
  immediately.
- Health badges include `role="status"` with a textual label so they are
  not colour-only.
- Dialogs trap focus and close on Escape (Radix Dialog primitives).

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| Indexer stuck on **Unknown** | Health worker hasn't run yet, or the indexer is disabled. Try **Enable** + wait one probe interval. |
| **Failed** badge with `dial tcp` error | URL unreachable or DNS failure. Check the URL and any attached proxy. |
| Cannot delete a proxy (`proxy_in_use`) | One or more indexers reference it. Edit those indexers and set **Proxy → None** first. |
| `unknown_kind` error on save | OpenAPI rejects unknown indexer/proxy kinds; pick one of the supported values listed above. |
