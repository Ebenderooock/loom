# Deployment

Loom ships as a single static binary and as multi-arch container images.
Choose an installation pathway below. Native installers are shipped inside
platform release archives by the updated release pipeline; **older releases do
not gain these files retroactively**. Until a release includes them, use this
checkout's scripts with a compatible binary. Windows requires a service-aware
binary built from this source; older binaries without the `service` command
cannot be installed as a native Windows service.

## Installation pathways

| Host | Installer / deployment | Guide |
|---|---|---|
| Windows (x64, ARM64) | ZIP with `loom.exe` and PowerShell Windows service installer | [Windows](windows.md) |
| Linux (amd64, ARM64, ARMv7) | Static binary archive and systemd installer | [Linux](linux.md) |
| macOS (Intel, Apple Silicon) | Static binary archive and launchd installer | [macOS](macos.md) |
| Raspberry Pi | Raspberry Pi OS + Linux installer, or ARM64 container | [Raspberry Pi](raspberry-pi.md) |
| Docker / Compose | GHCR image and standalone Compose file | [Docker](docker.md) |
| Kubernetes | Repository Helm chart with persistent storage | [Kubernetes](kubernetes.md) |
| Kubernetes with Flux | GitRepository + HelmRelease example | [Short Flux setup](flux.md) |
| Synology DSM | Container Manager, locally built `.spk` upload, or SynoCommunity once published | [Synology](synology.md) |

The archives and scripts are not signed MSI, macOS PKG/DMG, DEB, or RPM packages.
Synology native packages require a matching spksrc build; catalog publication is
a separate upstream process. Containers currently support Linux amd64 and ARM64
only. Other operating systems are not published release targets; source builds
on them are outside these installation paths.

### Before installing

- Choose a release from [GitHub Releases](https://github.com/ebenderooock/loom/releases).
  Check `checksums.txt` against the exact archive before executing its binary or
  installer; checksums detect corruption but are not publisher signatures.
- Persist config, database, and caches; set `LOOM_CONFIG_DIR`, `LOOM_DATA_DIR`,
  and `LOOM_STORAGE_SQLITE_PATH` explicitly when writing your own service.
- Grant the service account access to the media and download folders. Both Loom
  and the download client must see compatible paths; use a shared filesystem for
  hardlinks. Do not grant world-write access to work around permissions.
- Keep SQLite on local storage, with one Loom instance per database. Use the
  same single-instance deployment model for Postgres: the scheduler is not
  distributed just because the database is remote.
- Open `http://<host>:1925`, complete initial administrator setup, and use TLS
  via a reverse proxy for remote access. Do not expose an unconfigured instance
  directly to the Internet.

### Upgrades and backups

Stop Loom before copying the complete state directory, including SQLite WAL/SHM
files if present, or use a consistent database backup. Keep media separately
backed up. Pin the binary version, container tag/digest, and Helm source revision.
After upgrading, check `/readyz` and the UI. Database migrations are not undone
by replacing a binary: rollback needs the old version **and** its pre-upgrade
database backup. Each platform guide covers service logs and removal without
discarding state.

## Docker

### Prerelease stream

Experimental work lands through PRs into `prelease/0.3.1`, not `main`.
CI runs on PRs and pushes to `prelease/**`. After a code change is merged,
the separate prerelease publisher builds Linux amd64 images under:

- `ghcr.io/ebenderooock/loom:prelease-0.3.1` (moving opt-in channel).
- `ghcr.io/ebenderooock/loom:prelease-0.3.1-<sha>-<run>-<attempt>`
  (unique build tag for pinning/rollback).

Flux image automation tracks only exact `prelease-X.Y.Z` tags and orders
them by semver. Rebuilding the same branch updates the moving tag in place, so
cut a new `prelease/<next patch>` branch when a build should roll out.

The branch spelling is deliberately `prelease`, matching the channel name.
Publication can also be retried using **Publish prerelease image → Run
workflow**, selecting that branch. It only runs on `prelease/<semver>`
branches. Documentation-only pushes do not publish images.

This workflow never writes `latest`, `main-*`, or stable version tags,
creates a GitHub release, or changes a deployment. Existing stable/main
consumers stay on their selected channel; test deployments must explicitly
opt in. CI results should be checked before deploying a candidate.

Run a **separate** instance with its own database/config directory, port,
and test media/download locations. Never share a live SQLite database,
Postgres schema, or writable media/download paths with production; a
prerelease may migrate data or trigger automation, and switching images
back does not undo a migration. Back up before promoting any candidate.

```bash
docker run --rm -p 1926:1925 \
  -v /opt/loom-prelease/config:/config \
  ghcr.io/ebenderooock/loom:prelease-0.3.1
```

The usual release-tag workflow already supports tags such as
`v0.2.9-pre.1`: these produce GitHub prereleases and versioned images
without advancing `latest`. Creating/pushing a release tag is a separate,
explicit maintainer action, not an automatic consequence of this branch.

### Stable images

The published image lives at `ghcr.io/ebenderooock/loom:latest` and
`:<semver>` (without a `v` prefix). It is built `FROM
gcr.io/distroless/static-debian12:nonroot`, runs as the unprivileged
`nonroot` user, and exposes port 1925.

### Single container

```bash
docker compose -f deploy/docker/compose.yaml up -d
```

Run from this checkout after following the [Docker guide](docker.md) to create
the state and shared data directories with the correct ownership. `/config`
holds persistent state, and `/media` holds downloads and media at matching paths
in Loom and the download client.

### Docker Compose (development & demos)

The repo ships [`docker-compose.yml`](../docker-compose.yml) wiring Loom
together with qBittorrent, Prometheus, and Grafana. Optional Postgres is
commented out in the file:

```bash
docker compose up -d
# UIs:
#  Loom        http://localhost:1925
#  qBittorrent http://localhost:8080
#  Prometheus  http://localhost:9090
#  Grafana     http://localhost:3000  (admin/admin)
```

Switch Loom to Postgres by uncommenting the `postgres` service block
and setting `LOOM_DATABASE_URL=postgres://loom:loom@postgres:5432/loom?sslmode=disable`.

### Health probes

Use `loom healthcheck` to probe `/healthz`; it works without a shell or curl
inside the distroless image. Override with
`LOOM_HEALTH_URL=http://127.0.0.1:1925` if you've changed the listen address.
The Compose example configures a health check; Kubernetes uses HTTP probes.

## Kubernetes

The [Helm chart](../deploy/helm/loom/) already exists and can be installed from
this checkout. Follow the [Kubernetes guide](kubernetes.md) for storage,
permissions, probes, ingress, and upgrades, or the [Flux guide](flux.md) for
Git-managed installation. No published Helm repository or OCI chart is required.

## Bare-metal binary

GoReleaser (`.goreleaser.yaml`) produces checksummed archives for Linux, macOS,
and Windows. Use the [Linux systemd](linux.md), [macOS launchd](macos.md), or
[Windows service](windows.md) installer rather than copying a service unit with
implicit working-directory-dependent configuration.

## Reverse proxy

Loom listens on plain HTTP. Terminate TLS at the edge.

### Traefik (Docker labels)

```yaml
services:
  loom:
    image: ghcr.io/ebenderooock/loom:latest
    labels:
      - traefik.enable=true
      - traefik.http.routers.loom.rule=Host(`loom.example.com`)
      - traefik.http.routers.loom.entrypoints=websecure
      - traefik.http.routers.loom.tls.certresolver=letsencrypt
      - traefik.http.services.loom.loadbalancer.server.port=1925
```

### Caddy

```caddyfile
loom.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:1925
}
```

### Nginx

```nginx
server {
  listen 443 ssl http2;
  server_name loom.example.com;

  ssl_certificate     /etc/letsencrypt/live/loom.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/loom.example.com/privkey.pem;

  location / {
    proxy_pass http://127.0.0.1:1925;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade           $http_upgrade;
    proxy_set_header Connection        "upgrade";
  }
}
```

When the proxy mounts Loom under a path prefix, set
`http.url_base: /loom` so generated links are correct.

## Observability wiring

- Prometheus: scrape `/metrics` — see
  [`deploy/prometheus.yml`](../deploy/prometheus.yml).
- OpenTelemetry: point an OTel collector at `otel.endpoint`; export from
  the collector to your tracing backend of choice. See
  [observability.md](observability.md).

## Reference deployment topology

```text
[ users ]──TLS──▶ [ Traefik / Caddy ]──HTTP──▶ [ Loom ]──┬──▶ [ SQLite / Postgres ]
                                                         ├──▶ [ NATS (split-mode) ]
                                                         └──▶ [ OTel Collector ]──▶ [ Tempo / Grafana / … ]

                                          [ Prometheus ]──scrape──▶ [ Loom:/metrics ]
```
