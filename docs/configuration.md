# Configuration

Loom reads configuration in layered order; each layer overrides the previous:

1. Built-in defaults (in `internal/kernel/config/config.go`).
2. YAML file at `$LOOM_CONFIG_DIR/loom.yaml` (or the explicit `--config <path>`).
3. Environment variables (`LOOM_*`).
4. Command-line flags on `loom serve`.

Hot-reload is opt-in via `hot_reload: true`. Only **safe** keys are honoured
when a reload event fires (currently `log.level` and `log.format`); changing
a non-safe key in the YAML file at runtime is logged and ignored until the
next restart.

> **Auth keys.** The `auth.*` block is being implemented in Phase 1c and is
> intentionally omitted from the table below. It will be documented in
> [auth.md](auth.md) once the Phase 1c PR lands.

## Environment variable mapping

The viper binding maps a YAML path `a.b.c` to env var `LOOM_A_B_C`. A few
keys also accept legacy un-prefixed names for backwards compatibility with
pre-Viper deployments; these are noted in the Env column.

## Reference (Phase 1a)

| YAML path | Env var | Type | Default | Hot-reload | Description |
|---|---|---|---|---|---|
| `config_dir` | `LOOM_CONFIG_DIR` | string | `$PWD/config` | no | Where the YAML file is read from. |
| `data_dir` | `LOOM_DATA_DIR` | string | `$PWD/config` | no | Persistent state root (DB file, caches, logs). |
| `hot_reload` | `LOOM_HOT_RELOAD` | bool | `false` | no | Enable file-watcher hot-reload for safe keys. |
| `http.addr` | `LOOM_HTTP_ADDR` | string | `:8989` | no | HTTP listen address. |
| `http.read_timeout` | `LOOM_HTTP_READ_TIMEOUT` | int (s) | `30` | no | HTTP read header timeout. |
| `http.write_timeout` | `LOOM_HTTP_WRITE_TIMEOUT` | int (s) | `60` | no | HTTP write timeout. |
| `http.shutdown_timeout` | `LOOM_HTTP_SHUTDOWN_TIMEOUT` | int (s) | `30` | no | Graceful shutdown deadline. |
| `http.url_base` | `LOOM_HTTP_URL_BASE` | string | `""` | no | URL prefix when reverse-proxied (e.g. `/loom`). |
| `log.level` | `LOOM_LOG_LEVEL` | string | `info` | **yes** | One of `debug`, `info`, `warn`, `error`. |
| `log.format` | `LOOM_LOG_FORMAT` | string | `json` | **yes** | One of `json`, `text`. |
| `telemetry.prometheus` | `LOOM_TELEMETRY_PROMETHEUS` / `LOOM_PROMETHEUS` | bool | `true` | no | Expose `/metrics`. |
| `telemetry.profiling` | `LOOM_TELEMETRY_PROFILING` / `LOOM_PROFILING` | bool | `false` | no | Synonym kept for legacy callers; see also `debug.pprof`. |
| `telemetry.trace_ratio` | `LOOM_TELEMETRY_TRACE_RATIO` / `LOOM_TRACE_RATIO` | float [0,1] | `0.0` | no | Sampler ratio for OTel traces. |
| `telemetry.otlp_endpoint` | `LOOM_TELEMETRY_OTLP_ENDPOINT` / `LOOM_OTLP_ENDPOINT` | string | `""` | no | OTLP/HTTP collector endpoint. |
| `database.url` | `LOOM_DATABASE_URL` | string | `""` | no | Legacy-style DSN; if set and `postgres://…`, selects Postgres. |
| `storage.engine` | `LOOM_STORAGE_ENGINE` | string | `sqlite` | no | One of `sqlite`, `postgres`. |
| `storage.sqlite.path` | `LOOM_STORAGE_SQLITE_PATH` | string | `/data/loom.db` | no | Path to the SQLite file. |
| `storage.postgres.dsn` | `LOOM_STORAGE_POSTGRES_DSN` | string | `""` | no | `postgres://user:pass@host:5432/loom?sslmode=…` |
| `debug.pprof` | `LOOM_DEBUG_PPROF` | bool | `false` | no | Mount `/debug/pprof/*`. |
| `cors.allowed_origins` | `LOOM_CORS_ALLOWED_ORIGINS` | []string | `[]` | no | Allow-list for the chi-cors middleware. |
| `otel.enabled` | `LOOM_OTEL_ENABLED` | bool | `false` | no | Enable the OpenTelemetry SDK. |
| `otel.endpoint` | `LOOM_OTEL_ENDPOINT` | string | `""` | no | Same role as `telemetry.otlp_endpoint`; explicit OTel block. |

### Validation rules

The loader rejects configurations that fail any of the following at start-up:

- `log.level` ∈ {debug, info, warn, error}
- `log.format` ∈ {json, text}
- `http.addr` non-empty
- `telemetry.trace_ratio` ∈ [0, 1]
- `auth.mode` ∈ {forms, apikey, oidc, proxy, disabled}
- `storage.engine` ∈ {"", sqlite, postgres}

## YAML example

```yaml
# loom.yaml — minimal example
config_dir: /config
data_dir: /config
hot_reload: true

http:
  addr: ":8989"
  url_base: ""

log:
  level: info
  format: json

telemetry:
  prometheus: true
  trace_ratio: 0.0
  otlp_endpoint: ""

storage:
  engine: sqlite
  sqlite:
    path: /config/loom.db

cors:
  allowed_origins: []

debug:
  pprof: false
```

## Adding a new config key

See [contributing-style.md](contributing-style.md#documentation-requirements);
every new or changed key must land with an update to this page.
