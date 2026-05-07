# Deployment

Loom ships as a single static binary and as multi-arch container images.
This page covers the supported deployment shapes.

## Docker

The published image lives at `ghcr.io/ebenderooock/loom:latest` (and
`:v<semver>` once tagged releases begin). It is built `FROM
gcr.io/distroless/static-debian12:nonroot`, runs as the unprivileged
`nonroot` user, and exposes port 8989.

### Single container

```bash
docker run --rm -p 8989:8989 \
  -v /opt/loom/config:/config \
  -v /mnt/media:/media \
  ghcr.io/ebenderooock/loom:latest
```

Volumes:

- `/config` — Loom's config + state (DB file, caches). Persist this.
- `/media` — your media library; mount at the same path the download
  client sees so hardlinks work.

### Docker Compose (development & demos)

The repo ships [`docker-compose.yml`](../docker-compose.yml) wiring Loom
together with qBittorrent, Prometheus, and Grafana. Optional Postgres is
commented out in the file:

```bash
docker compose up -d
# UIs:
#  Loom        http://localhost:8989
#  qBittorrent http://localhost:8080
#  Prometheus  http://localhost:9090
#  Grafana     http://localhost:3000  (admin/admin)
```

Switch Loom to Postgres by uncommenting the `postgres` service block
and setting `LOOM_DATABASE_URL=postgres://loom:loom@postgres:5432/loom?sslmode=disable`.

### Health probes

The image's `HEALTHCHECK` runs `loom healthcheck`, which probes
`/healthz`. Override with `LOOM_HEALTH_URL=http://127.0.0.1:8989` if
you've changed the listen address.

## Kubernetes

A Helm chart (`deploy/helm/loom/`) and a Kustomize base + overlays
(`deploy/kustomize/`) are scheduled for **Phase 11**. The chart will
expose values for ingress, persistence, OIDC, Postgres, NATS, OTel
collector. Until then, a minimal Deployment + Service + PVC manifest is
straightforward to write against the container above.

## Bare-metal binary

GoReleaser (`.goreleaser.yaml`) produces signed binaries for Linux,
macOS, Windows, and FreeBSD across `amd64`, `arm64`, and `armv7`.
Download from the releases page, drop into `/usr/local/bin`, and run
under your favourite supervisor:

```ini
# /etc/systemd/system/loom.service
[Unit]
Description=Loom
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/loom serve --config /etc/loom/loom.yaml
User=loom
Group=loom
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/loom /etc/loom
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

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
      - traefik.http.services.loom.loadbalancer.server.port=8989
```

### Caddy

```caddyfile
loom.example.com {
  encode zstd gzip
  reverse_proxy 127.0.0.1:8989
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
    proxy_pass http://127.0.0.1:8989;
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

## Reference deployment topology (Phase 11 target)

```text
[ users ]──TLS──▶ [ Traefik / Caddy ]──HTTP──▶ [ Loom ]──┬──▶ [ Postgres ]
                                                         ├──▶ [ NATS (split-mode) ]
                                                         └──▶ [ OTel Collector ]──▶ [ Tempo / Grafana / … ]

                                          [ Prometheus ]──scrape──▶ [ Loom:/metrics ]
```
