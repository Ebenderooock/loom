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

> **Auth keys.** The full `auth.*` block landed with Phase 1c (commit
> `3f28c1c`); see [auth.md](auth.md) for the auth-specific reference.
> Core kernel/transport/storage keys live below.

## Environment variable mapping

The viper binding maps a YAML path `a.b.c` to env var `LOOM_A_B_C`. A few
keys also accept legacy un-prefixed names for backwards compatibility with
pre-Viper deployments; these are noted in the Env column.

## Reference (Phase 1a)

The table is the at-a-glance view; the per-key examples below show what
real values look like. Every key gets an `example:` line.

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
| `scheduler.enabled` | `LOOM_SCHEDULER_ENABLED` | bool | `true` | no | Master switch for the in-process job scheduler. |
| `scheduler.timezone` | `LOOM_SCHEDULER_TIMEZONE` | string | `Local` | no | IANA name (e.g. `UTC`, `Europe/Stockholm`) used to interpret cron expressions. |
| `scheduler.shutdown_grace` | `LOOM_SCHEDULER_SHUTDOWN_GRACE` | int (s) | `30` | no | Seconds in-flight handlers may keep running after `SIGTERM`. |
| `indexers.search_timeout` | `LOOM_INDEXERS_SEARCH_TIMEOUT` | int (s) | `15` | no | Per-indexer ceiling for fan-out search calls. |
| `indexers.max_parallel` | `LOOM_INDEXERS_MAX_PARALLEL` | int | `8` | no | Maximum number of concurrent indexer Search calls during fan-out. |
| `indexers.health_check_schedule` | `LOOM_INDEXERS_HEALTH_CHECK_SCHEDULE` | string (cron) | `*/10 * * * *` | no | 5-field cron expression (no seconds field) controlling the periodic health sweep. |
| `indexers.health_check_timeout` | `LOOM_INDEXERS_HEALTH_CHECK_TIMEOUT` | int (s) | `10` | no | Per-indexer ceiling for the periodic health check `Test()` call. |

### Per-key examples

```yaml
config_dir: /config                     # example: where loom.yaml lives
data_dir: /config                       # example: writable state directory
hot_reload: true                        # example: watch loom.yaml for safe-key changes

http:
  addr: ":8989"                         # example: bind all interfaces, port 8989
  read_timeout: 30                      # example: 30s for slow uploaders
  write_timeout: 60                     # example: 60s ceiling for streaming responses
  shutdown_timeout: 30                  # example: 30s grace on SIGTERM
  url_base: "/loom"                     # example: behind a /loom path prefix

log:
  level: info                           # example: debug | info | warn | error
  format: json                          # example: json (prod) | text (dev)

telemetry:
  prometheus: true                      # example: keep /metrics on
  profiling: false                      # example: legacy alias; prefer debug.pprof
  trace_ratio: 0.05                     # example: sample 5% of traces
  otlp_endpoint: "http://otel:4318"     # example: OTLP/HTTP collector

database:
  url: "postgres://loom:loom@db:5432/loom?sslmode=disable"  # example: legacy alias for storage.postgres.dsn

storage:
  engine: sqlite                        # example: sqlite | postgres
  sqlite:
    path: /config/loom.db               # example: single-file, container-friendly
  postgres:
    dsn: "postgres://loom:loom@db:5432/loom?sslmode=require" # example: prod with TLS

debug:
  pprof: true                           # example: enable /debug/pprof/* (gate behind a private network)

cors:
  allowed_origins:                      # example: front-end on a different host
    - "https://media.example.com"

otel:
  enabled: true                         # example: turn the OTel SDK on
  endpoint: "http://otel:4318"          # example: same as telemetry.otlp_endpoint

scheduler:
  enabled: true                         # example: run the cron scheduler
  timezone: "Europe/Stockholm"          # example: IANA name; "Local" follows host TZ
  shutdown_grace: 30                    # example: seconds to let jobs finish on SIGTERM

indexers:
  search_timeout: 15                    # example: per-indexer fan-out ceiling, seconds
  max_parallel: 8                       # example: concurrent Search calls during a single fan-out
  health_check_schedule: "*/10 * * * *" # example: 5-field cron — every 10 minutes
  health_check_timeout: 10              # example: per-indexer Test() ceiling, seconds
```

### Validation rules

The loader rejects configurations that fail any of the following at start-up:

- `log.level` ∈ {debug, info, warn, error}
- `log.format` ∈ {json, text}
- `http.addr` non-empty
- `telemetry.trace_ratio` ∈ [0, 1]
- `auth.mode` ∈ {forms, apikey, oidc, proxy, disabled}
- `storage.engine` ∈ {"", sqlite, postgres}
- `scheduler.timezone` is a valid IANA name or the literal `Local`
- `scheduler.shutdown_grace` ≥ 0
- `indexers.search_timeout` > 0
- `indexers.max_parallel` > 0
- `indexers.health_check_schedule` parses as a 5-field cron expression
- `indexers.health_check_timeout` > 0

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
