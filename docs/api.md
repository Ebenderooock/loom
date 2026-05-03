# API

The canonical machine-readable specification of Loom's HTTP API lives in
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) (OpenAPI 3.1). This
page is the human summary.

## Versioning policy

| Path prefix | Owner | Stability |
|---|---|---|
| `/api/v1/*` | Loom-native | Active. Breaking changes only at major version bumps. |
| `/api/v3/*` | Wire-compat: Radarr v3 / Sonarr v3 + v4 | Lands in Phase 7. We track upstream shape; we do not extend it. |
| `/api/v1/*` (Prowlarr namespace) | Wire-compat: Prowlarr v1 | Lands in Phase 7. Disambiguated by the routes Prowlarr clients call. |

Wire-compat surfaces are explicitly _shape-compatible_, not
_semantically-extended_: clients that work against the upstream apps
should work against Loom unchanged.

## Routes (current)

These are the routes implemented today. They are the routes documented
in `api/openapi/loom.yaml`.

| Method | Path | Auth | Response | Description |
|---|---|---|---|---|
| `GET` | `/healthz` | none | `200 application/json` | Liveness probe. |
| `GET` | `/livez` | none | `200 application/json` | Same shape; conventional alias. |
| `GET` | `/readyz` | none | `200` or `503 application/json` | DB ping + ready-flag check. |
| `GET` | `/metrics` | none | `200 text/plain; version=0.0.4` | Prometheus exposition. |
| `GET` | `/api/v1/system/status` | none (today) | `200 application/json` (`SystemStatus`) | Build metadata. |
| `GET\|POST` | `/debug/pprof/*` | gated by `debug.pprof` | varies | Standard `net/http/pprof` handlers. |
| `GET` | `/api/v1/indexers/` | apiKey/bearer/cookie | `200 application/json` (`[]DefinitionWithHealth`) | List configured indexers with the latest health row. |
| `POST` | `/api/v1/indexers/` | apiKey/bearer/cookie | `201 application/json` (`Definition`) | Create an indexer. |
| `GET` | `/api/v1/indexers/{id}` | apiKey/bearer/cookie | `200 application/json` (`DefinitionWithHealth`) | Fetch a single indexer + its latest health row. |
| `PUT` | `/api/v1/indexers/{id}` | apiKey/bearer/cookie | `200 application/json` (`Definition`) | Replace an existing indexer's full definition. |
| `PATCH` | `/api/v1/indexers/{id}` | apiKey/bearer/cookie | `200 application/json` (`Definition`) | Partially update an existing indexer. |
| `DELETE` | `/api/v1/indexers/{id}` | apiKey/bearer/cookie | `204` | Remove an indexer. |
| `GET` | `/api/v1/indexers/{id}/caps` | apiKey/bearer/cookie | `200 application/json` (`Caps`) | Per-indexer capability descriptor. |
| `POST` | `/api/v1/indexers/{id}/test` | apiKey/bearer/cookie | `200 application/json` (`Health`) | Run an immediate health check; persists the result. |
| `POST` | `/api/v1/indexers/search` | apiKey/bearer/cookie | `200 application/json` (`AggregatedResults`) | Fan-out search across all (or a subset of) enabled indexers. |
| `GET` | `/api/v1/proxies/` | apiKey/bearer/cookie | `200 application/json` (`{"proxies":[Proxy]}`) | List configured outbound proxies. |
| `POST` | `/api/v1/proxies/` | apiKey/bearer/cookie | `201 application/json` (`Proxy`) | Create a proxy (HTTP/HTTPS/SOCKS5/FlareSolverr). |
| `GET` | `/api/v1/proxies/{id}` | apiKey/bearer/cookie | `200 application/json` (`Proxy`) | Fetch one proxy. |
| `PUT` | `/api/v1/proxies/{id}` | apiKey/bearer/cookie | `200 application/json` (`Proxy`) | Replace an existing proxy's full definition. |
| `PATCH` | `/api/v1/proxies/{id}` | apiKey/bearer/cookie | `200 application/json` (`Proxy`) | Partially update a proxy. |
| `DELETE` | `/api/v1/proxies/{id}` | apiKey/bearer/cookie | `204` or `409 application/json` | Remove a proxy. Returns 409 `proxy_in_use` (with `details.indexer_ids`) when any indexer still references it. |
| `POST` | `/api/v1/proxies/{id}/test` | apiKey/bearer/cookie | `200 application/json` (`ProxyTestResult`) | Probe the proxy: HTTP/SOCKS5 fetch the configured probe URL; FlareSolverr issues `sessions.list`. |

`SystemStatus` is `{ version, commit, buildDate, engine }` — the database
engine name is included so readiness tooling can confirm what backend
this instance is running against.

### Try every route with curl

Boot the binary on the default port (`make build && ./dist/loom serve`)
or hit your container, then:

```bash
$ curl -s http://localhost:8989/healthz
{"status":"ok"}

$ curl -s http://localhost:8989/livez
{"status":"alive"}

$ curl -s http://localhost:8989/readyz
{"status":"ready"}        # or HTTP 503 with {"status":"not ready","reason":"..."}

$ curl -s http://localhost:8989/api/v1/system/status
{"buildDate":"2026-05-01T02:27:12Z","commit":"e561225","engine":"sqlite","version":"e561225"}

$ curl -s http://localhost:8989/metrics | head -3
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="github.com/loomctl/loom",version="..."} 1

# pprof is gated by debug.pprof: true
$ curl -s http://localhost:8989/debug/pprof/goroutine?debug=1 | head -1
goroutine profile: total 9
```

## Headers, middleware, and conventions

- Every response includes `X-Request-Id` (echoed from the request when
  present, otherwise generated by chi's `RequestID` middleware).
- Responses are gzip-compressed when the client signals `Accept-Encoding`.
- `/api/v1/system/status` is wrapped with an `ETag` middleware for
  cheap polling.
- CORS is **off** by default; populate `cors.allowed_origins` to enable.

## Authentication (incoming, Phase 1c)

The OpenAPI spec defines three security schemes ahead of the handlers:

- `apiKey` — header `X-Api-Key`.
- `bearerAuth` — HTTP `Authorization: Bearer <token>`.
- `cookieAuth` — cookie `loom_session`.

The endpoints that consume them land with the Phase 1c auth PR. See
ADR-0004 and [auth.md](auth.md).

## Indexers (Phase 2a)

The indexer routes accept and emit JSON. Conceptual reference lives
in [indexers.md](indexers.md); routes are summarised below.

```bash
# Create
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"id":"demo","kind":"builtin/null","name":"Demo","enabled":true,"priority":25,"categories":[2000]}' \
  http://localhost:8989/api/v1/indexers/

# List
curl -sS -H "X-Api-Key: $LOOM_KEY" http://localhost:8989/api/v1/indexers/

# Manual health check
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:8989/api/v1/indexers/demo/test

# Fan-out search
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"query":"ubuntu","categories":[4000]}' \
  http://localhost:8989/api/v1/indexers/search
```

### Newznab + Torznab (Phase 2c)

Loom registers two real source kinds, `newznab` and `torznab`, in
addition to `builtin/null`. They share a single config schema and
parser; only the extended-attribute namespace differs. Full reference
lives in [indexers-newznab.md](indexers-newznab.md).

```bash
# Create a Newznab indexer (e.g. NZBHydra2 or Prowlarr)
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "id": "hydra-news",
    "kind": "newznab",
    "name": "NZBHydra2",
    "enabled": true,
    "priority": 50,
    "config": {
      "url": "https://nzbhydra.example/api",
      "api_key": "abcdef0123456789",
      "timeout": "20s"
    },
    "categories": [2000, 5000, 7000]
  }' \
  http://localhost:8989/api/v1/indexers/

# Probe caps (cached to indexer_health.last_caps_json)
curl -sS -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:8989/api/v1/indexers/hydra-news/caps
```

### Proxies (Phase 2e)

Indexers may pin outbound traffic through a proxy row. Conceptual
reference lives in [indexers-proxies.md](indexers-proxies.md). All
routes share the indexer auth scope.

```bash
# Create an HTTP proxy
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{
    "kind":"http","name":"Squid","enabled":true,
    "config":{"url":"http://squid.lan:3128","username":"u","password":"p"}
  }' http://localhost:8989/api/v1/proxies/

# Pin an existing indexer to that proxy
curl -sS -X PATCH -H "X-Api-Key: $LOOM_KEY" -H "Content-Type: application/json" \
  -d '{"proxy_id":"http-squid"}' http://localhost:8989/api/v1/indexers/hydra-news

# Test the proxy
curl -sS -X POST -H "X-Api-Key: $LOOM_KEY" \
  http://localhost:8989/api/v1/proxies/http-squid/test
```

### Error envelope

Indexer endpoints reply to 4xx/5xx responses with a structured
envelope:

```json
{ "error": { "code": "not_found", "message": "indexer not found" } }
```

This is **not** the same shape used by the auth package (which today
emits `{"error":"<msg>"}`). Clients should branch on response status
before parsing the body, or accept either shape via
`{ "error": <string|{"code":"...","message":"..."}> }`. The
discrepancy is tracked for unification in a later phase; the
indexer envelope is the long-term direction.

## Adding a route

1. Author the handler in `internal/server/` (or under the relevant
   module package).
2. Update [`api/openapi/loom.yaml`](../api/openapi/loom.yaml) — _every_ new
   route must be in the spec.
3. Re-run `redocly lint api/openapi/loom.yaml` (or
   `swagger-cli validate`).
4. Update this page if the change touches the route table or auth model.

The doc-update policy is enforced by the PR template — see
[contributing-style.md](contributing-style.md#documentation-requirements).
