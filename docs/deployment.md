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

### Update channels

**Settings > General > Updates** defaults to Stable. Prelease requires an
explicit risk acknowledgement and a saved opt-in. Signed notices need a
publisher public key compiled into Loom and matching published signed manifests.
Until signed manifests are available, use GitHub Releases manually.
Restricted Linux/systemd and macOS/launchd helpers implement stopped-service
SQLite/config snapshots, health verification and rollback for canonical native
installs. Production activation still needs live platform
upgrade/reboot validation; Windows remains unsupported. PostgreSQL stays manual-only;
containers and Kubernetes continue using image/orchestration updates.
See [native updates](native-updates.md) for the current limits, signing contract,
native branch-build artifacts, and separate publisher work.

## Trusted publishing

Production publishing is owned by the public
[`Ebenderooock/loom-publisher`](https://github.com/Ebenderooock/loom-publisher)
repository. Loom's publishing workflows have read-only tokens: they wait for
the publisher's GitHub App to report the actual result on the source commit.
The public CI and publishing badges, GitHub releases, and downloads remain in
this repository. Dispatching or scheduling a build is not reported as a
successful publication.

The publisher validates successful source CI and allowed source refs, then
builds the verified commit on a runner without publishing credentials.
Separate trusted jobs publish the outputs; they must not execute source
scripts or load source-controlled publishing configuration.

The supported streams are:

| Source ref | Published output |
|---|---|
| `main` | Linux amd64 image, `main-<sha>-<commit timestamp>` |
| `prelease/X.Y.Z` | Linux amd64 images, `prelease-X.Y.Z` and a pinned build tag |
| `prerelease/X.Y.Z` | Linux amd64 images, `prerelease-X.Y.Z` and a pinned build tag |
| `vX.Y.Z` on `main` history | Public release assets, versioned amd64/arm64 image, and stable `latest` |
| Semver prerelease tag on `main` history | Public prerelease assets and versioned images, never stable `latest` |

### Cutting a release

A prerelease branch is a rolling channel; a version tag cuts a release.

1. To start an experimental line, choose `X.Y.Z` and create
   `prelease/X.Y.Z` from `main`. The active **Restrict publishing branch
   creation** ruleset blocks new `prelease/**` and `prerelease/**` branches
   and has no bypass actor. A repository ruleset owner must temporarily
   exclude only the exact ref (for example, `refs/heads/prelease/0.4.6`),
   create it from `main`, and then remove that exception. Keep publishing
   branch protection enabled. Experimental changes should land through PRs
   targeting this branch.
2. On code pushes to the branch, the prerelease workflow builds unsigned
   native archives and waits for the isolated publisher; documentation-only
   pushes are ignored. The source workflow has no signing or publishing
   credentials. The publisher validates the source ref, commit, and CI result
   before publishing its configured outputs.
3. When the line is ready, merge it into `main` through a PR. After required
   checks pass, create the matching `vX.Y.Z` tag on the resulting `main`
   commit. The **Restrict release tag creation** ruleset blocks new `v*` tags;
   a repository ruleset owner must temporarily exclude only the exact tag
   (for example, `refs/tags/v0.4.6`) to create it, then remove the exception.
   The **Keep release tags immutable** ruleset prevents moving or deleting it.
4. The tag starts `release.yml`, which only observes the publisher's result.
   The isolated publisher verifies the eligible source and performs the
   privileged release. Stable tags advance `latest`; SemVer prerelease tags
   such as `vX.Y.Z-pre.N` publish as prereleases and never advance `latest`.

### Owner setup and cutover

The publisher stays disabled until its owner completes setup. Public
repositories support the necessary protections on GitHub Free.

1. Restrict publisher write access to release maintainers. Protect its `main`
   branch with required PR and CODEOWNER approval, dismissed stale approvals,
   required checks, and blocked deletion/force pushes. Restrict bypass.
   Protect Loom's `main`, `prelease/**`, and `prerelease/**` branches and
   release tags too. Source CI must pass before publishing.
2. In the publisher, create environment `production-publish`, allowing only
   the **branch** `main`, with no permitted tags. Disallow administrator
   bypass. PR approval can authorize routine automatic publishing; optionally
   require environment reviewers for approval of each publishing run.
3. Use a dedicated publishing identity with Write access only to the intended
   GHCR package. Store its classic PAT with `write:packages` as environment
   secret `GHCR_PUBLISH_TOKEN`, and set publisher variable
   `GHCR_PUBLISH_USERNAME` to that identity's username. Do not store the token
   in source repository secrets or ordinary publisher repository secrets.
4. Create a GitHub App installed only on `Ebenderooock/loom`: Contents
   read/write for releases, Commit statuses read/write for public results,
   Actions read for source CI validation, and Checks read for push-diff
   metadata. Store its App ID as publisher
   variable `LOOM_APP_ID` and its private key as environment secret
   `LOOM_APP_PRIVATE_KEY`. Set Loom repository variable `PUBLISHER_APP_SLUG`
   to the actual App slug, without `[bot]`. Never put the key in this
   repository. Configure the publisher's optional `TVDB_APIKEY` build secret
   separately if the image should embed the existing provider key.
   Set publisher variable `PUBLISHING_NOT_BEFORE` to the intended cutover UTC
   timestamp (`YYYY-MM-DDTHH:MM:SSZ`); older CI runs are excluded to avoid
   rebuilding historical releases.
5. Once the publisher workflows and their checks are ready, coordinate the
   cutover with the source workflow changes. In the GHCR package settings,
   disable inherited repository permissions and remove unintended user
   Write grants. Remove Actions Write/Admin access for **both** Loom and
   the publisher, and any other repository that should not publish.
   Publication uses the protected environment credential, not either
   repository's `GITHUB_TOKEN`. Keep public image pulls enabled.
6. Enable the publisher with its `PUBLISHING_ENABLED` repository variable
   set to `true`. Verify each stream, release assets, public statuses, and
   negative tests: an unapproved branch or a workflow requesting
   `packages: write` must not be able to publish production images.

Do not grant automatic package-write access to the publisher repository as
a shortcut: that would also authorize its unapproved branch workflows.
Environment protection is effective only when publishing credentials cannot
be obtained through another route. Existing publishing remains in effect
until these source workflow changes are merged; do not revoke its access
before the replacement and cutover are ready.

Documentation-only branch pushes do not publish automatically; explicit
publisher dispatch may publish them after successful CI. New branches and
pushes reaching GitHub's comparison-file limit conservatively publish.
Existing public releases are never overwritten; failed attempts require an
explicit publisher retry or a new CI attempt. Refer to the publisher's
[operator guide](https://github.com/Ebenderooock/loom-publisher#owner-setup--cutover-administrative-access-required)
for activation, retries, and scheduled-workflow monitoring.

### Release-page notes

Before preparing another release version or creating its tag, require successful
`main` push CI for the exact source commit across Linux, macOS and Windows.
PR CI also runs all three platforms; require successful Windows PR execution
before further release-version preparation. Cross-compilation is not Windows
runtime validation.
Tag CI retains the full matrix as an additional safeguard; do not move or reuse
an existing tag to recover a failure. The trusted publisher's authoritative
main-push CI authorization now includes the full platform suite.

Before creating a release tag, add a matching version section to
`CHANGELOG.md`, for example `## [0.4.2] - 2026-11-17` for `v0.4.2`.
Leave a fresh `## [Unreleased]` section for future work. Describe user-facing
changes since the previous release, including upgrade considerations;
do not carry forward already-shipped features as new changes.

The trusted publisher reads only that version's section from the exact
source commit selected for the tag. It uses those notes for the GitHub
release description, not the release PR body or generated commit notes.
Missing, empty or duplicate version sections fail publication. Prerelease
tags require their exact version section too, such as `[0.4.3-rc.1]`.
Merge the publisher's changelog-note support before tagging a release that
depends on this behavior. Release archives continue to include the full
`CHANGELOG.md`.

## Docker

### Prerelease stream

Experimental work lands through PRs into branches such as `prelease/0.3.1`,
not `main`. CI runs on PRs and pushes to both `prelease/**` and
`prerelease/**`. After a code change is merged and passes CI, the trusted
publisher builds Linux amd64 images under:

- `ghcr.io/ebenderooock/loom:prelease-0.3.1` (moving opt-in channel).
- `ghcr.io/ebenderooock/loom:prelease-0.3.1-<sha>-<run>-<attempt>`
  (unique build tag for pinning/rollback; run and attempt identify source CI).

Flux image automation tracks only exact `prelease-X.Y.Z` tags and orders
them by semver. Rebuilding the same branch updates the moving tag in place, so
cut a new `prelease/<next patch>` branch when a build should roll out.

The existing branch spelling `prelease` continues to work.
`prerelease/X.Y.Z` is also supported and publishes its own
`prerelease-X.Y.Z` channel instead of overwriting `prelease-X.Y.Z`.
The publisher periodically discovers eligible successful CI runs. Use its
manual workflow with the full source ref to request publication explicitly;
the same CI and ref restrictions apply. Rerunning Loom's **Publish
prerelease image** workflow only observes the result; it does not hold a
publishing credential or authorize a new build.

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
