# Indexer outbound proxies

Loom can route an indexer's outbound HTTP traffic through a
configurable proxy. This is what you reach for when:

- the indexer is behind a Cloudflare interactive challenge — point
  it at a [FlareSolverr] companion;
- the indexer is geo-fenced and you want to tunnel through a
  residential SOCKS5 endpoint;
- your egress only allows traffic via a corporate HTTP CONNECT
  gateway.

Proxies are first-class records (`/api/v1/proxies/*`) so one
proxy row can serve many indexers and you can rotate credentials in
one place.

## Kinds

| Kind            | Use it for                                                         | Config schema             |
| --------------- | ------------------------------------------------------------------ | ------------------------- |
| `http`          | Plain HTTP CONNECT gateways.                                       | `ProxyHTTPConfig`         |
| `https`         | TLS-terminated HTTP CONNECT gateways.                              | `ProxyHTTPConfig`         |
| `socks5`        | SOCKS5 proxies (with or without `user/pass`).                      | `ProxySOCKS5Config`       |
| `flaresolverr`  | A running [FlareSolverr] instance that solves Cloudflare for you.  | `ProxyFlareSolverrConfig` |

### `http` / `https`

```json
{
  "url": "http://gateway.lan:3128",
  "username": "alice",
  "password": "hunter2"
}
```

`username` and `password` are optional. Anything else in the JSON is
rejected.

### `socks5`

```json
{
  "address": "tunnel.example.net:1080",
  "username": "alice",
  "password": "hunter2"
}
```

`address` is `host:port`; no scheme.

### `flaresolverr`

```json
{
  "url": "http://flaresolverr:8191",
  "max_timeout_ms": 60000,
  "session_mode": "shared"
}
```

- `url` is the FlareSolverr base URL (no trailing slash).
- `max_timeout_ms` is forwarded as the FlareSolverr `maxTimeout`.
  Defaults to the kernel `flaresolverr_default_timeout` (seconds).
- `session_mode: "shared"` makes Loom lazily create one FlareSolverr
  session per proxy row and reuse it across requests; this is the
  only way to amortise the JS challenge cost. `none` (or empty)
  starts a fresh browser context per request.

The FlareSolverr round-tripper only handles GETs (newznab/torznab
issue pure GETs). It POSTs `{"cmd":"request.get",...}` to
`<URL>/v1`, parses the `solution` envelope, and synthesises a Go
`*http.Response` with the body, headers, cookies, and User-Agent
that FlareSolverr captured.

## Attaching a proxy to an indexer

Indexers grow a nullable `proxy_id` FK. Three knobs:

```sh
# 1. Pin on creation:
curl -sX POST localhost:8080/api/v1/indexers/ \
  -H 'Content-Type: application/json' \
  -d '{"kind":"newznab","name":"hydra","proxy_id":"flaresolverr-prod","config":{...}}'

# 2. Pin or repoint after the fact:
curl -sX PATCH localhost:8080/api/v1/indexers/hydra \
  -H 'Content-Type: application/json' \
  -d '{"proxy_id":"socks5-vps"}'

# 3. Detach (route via the default transport):
curl -sX PATCH localhost:8080/api/v1/indexers/hydra \
  -H 'Content-Type: application/json' \
  -d '{"proxy_id":null}'
```

`proxy_id:null` and `proxy_id:""` are both accepted as "detach".

## CRUD examples

```sh
# create
curl -sX POST localhost:8080/api/v1/proxies/ \
  -H 'Content-Type: application/json' \
  -d '{
    "kind":"flaresolverr",
    "name":"flaresolverr-prod",
    "config":{"url":"http://flaresolverr:8191","session_mode":"shared"}
  }'

# list
curl -s localhost:8080/api/v1/proxies/

# probe
curl -sX POST localhost:8080/api/v1/proxies/flaresolverr-prod/test

# patch
curl -sX PATCH localhost:8080/api/v1/proxies/flaresolverr-prod \
  -H 'Content-Type: application/json' \
  -d '{"enabled":false}'

# delete (409 if any indexer still pins this proxy)
curl -sX DELETE localhost:8080/api/v1/proxies/flaresolverr-prod
```

`/test` runs a real probe: HTTP/HTTPS/SOCKS5 fetch the kernel-config
`test_probe_url` (default `https://example.com/`) through the
proxy, FlareSolverr issues a `sessions.list` command. The result
includes `ok`, `latency_ms`, and an HTTP `status_code`.

## Transport caching & invalidation

The proxies service caches one `http.RoundTripper` per proxy row in
memory. Whenever a proxy is `PUT`, `PATCH`-ed, or `DELETE`-d, that
single cache entry is invalidated; the next outbound call from any
indexer using the row rebuilds the transport (and, for FlareSolverr,
re-creates the shared session lazily). Indexers themselves don't
need to be restarted or reloaded.

## FlareSolverr docker-compose snippet

```yaml
services:
  flaresolverr:
    image: ghcr.io/flaresolverr/flaresolverr:latest
    container_name: flaresolverr
    environment:
      - LOG_LEVEL=info
      - TZ=UTC
    ports:
      - "8191:8191"
    restart: unless-stopped
```

Then create the proxy:

```sh
curl -sX POST localhost:8080/api/v1/proxies/ \
  -H 'Content-Type: application/json' \
  -d '{
    "kind":"flaresolverr",
    "name":"flaresolverr-prod",
    "config":{"url":"http://flaresolverr:8191","session_mode":"shared"}
  }'
```

## Troubleshooting

| Symptom                                                        | Likely cause                                                                                                  |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `POST /api/v1/proxies/` returns `400 invalid_proxy_config`     | Unknown JSON key in `config` (the parser is strict) or missing required field for the kind.                   |
| `DELETE /api/v1/proxies/{id}` returns `409 proxy_in_use`       | Inspect `error.details.indexer_ids` — detach those indexers (`PATCH proxy_id:null`) or delete them first.     |
| `/test` reports `ok:false` for FlareSolverr                    | `<URL>/v1` is unreachable — confirm the container is up and the URL is reachable from the Loom process.       |
| Indexer health flips to `failed` after attaching a proxy       | Run `POST /api/v1/proxies/{id}/test` to isolate; then `POST /api/v1/indexers/{id}/test`.                      |
| SOCKS5 fails with `socks connect ... unreachable network`      | `address` must be a TCP `host:port` — no `socks5://` scheme.                                                  |

## Kernel config

Two new keys live under `indexers.proxies` (see
[configuration.md](configuration.md)):

- `flaresolverr_default_timeout` — int seconds, applied as
  `maxTimeout` when a FlareSolverr proxy doesn't set its own.
- `test_probe_url` — URL fetched by `/test` for non-FlareSolverr
  proxies.

## See also

- [ADR-0009](adr/0009-indexer-proxies.md) — design rationale.
- [api.md](api.md) — full route reference.
- [`api/openapi/loom.yaml`](../api/openapi/loom.yaml) — OpenAPI 3.1 schemas.

[FlareSolverr]: https://github.com/FlareSolverr/FlareSolverr
