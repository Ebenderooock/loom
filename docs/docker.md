# Run Loom with Docker

Use the standalone [`deploy/docker/compose.yaml`](../deploy/docker/compose.yaml)
for a persisted Loom service, without the development monitoring stack or
bundled download clients. Install [Docker Engine](https://docs.docker.com/engine/install/)
or [Docker Desktop](https://docs.docker.com/desktop/) and Docker Compose **2.23.1
or newer** (the example uses inline config content).

## 1. Choose an image and prepare storage

Choose a published version from [Loom releases](https://github.com/ebenderooock/loom/releases).
Release Git tags use `vX.Y.Z`; GHCR container tags use **`X.Y.Z`, without `v`**.
The release image supports Linux amd64 and arm64. Pin a release instead of
`latest` for controlled upgrades. Do not assume the example chart appVersion
is the release you want.

From a Loom checkout, set your version and **absolute** host directories:

```bash
export LOOM_IMAGE_TAG=X.Y.Z    # replace with an actual published version
export LOOM_CONFIG_PATH=/srv/loom/config
export LOOM_MEDIA_PATH=/srv/media
sudo mkdir -p "$LOOM_CONFIG_PATH/data"
sudo mkdir -p "$LOOM_MEDIA_PATH/downloads" "$LOOM_MEDIA_PATH/movies" "$LOOM_MEDIA_PATH/tv"
sudo chown -R 65532:65532 "$LOOM_CONFIG_PATH"
sudo chmod 700 "$LOOM_CONFIG_PATH"
```

The distroless image runs as UID/GID **65532**, not root, and does not implement
`PUID`/`PGID`. Grant that identity read/write/traverse access to media and downloads
using your host's groups or ACLs. For **new, empty** media directories, you can
set their owner to `65532:65532`; do not recursively change ownership on an
existing NAS library. The same write permissions must work for the downloader's
identity. The Compose file refuses to silently create missing bind directories.
With rootless Docker or user-namespace remapping, map the container UID to the
corresponding host UID rather than assuming host UID 65532.

| Container path | Purpose |
| --- | --- |
| `/config` | Writable persistent configuration, including `loom.json` (admin setup) |
| `/config/data/loom.db` | Explicit SQLite database path, set in mounted kernel YAML |
| `/media/downloads` | Downloader output visible to Loom |
| `/media/movies`, `/media/tv` | Example library root folders configured in the UI |
| `/tmp` | Bounded ephemeral scratch space, not persistent state |

Keep `/config` on a **local filesystem** with working SQLite locks; do not put
the database on NFS/SMB. Host-mounted NAS media is fine: mount the NAS on the host
first, verify permissions and availability, then bind its directory into Loom.
Arrange host startup so the NAS is mounted before Docker starts this service;
otherwise an empty underlying mountpoint can look like an empty library.
Docker Desktop must have file-sharing access to your chosen host directories.

**Existing installations:** `/config/data/loom.db` is deliberately the same
database path as Loom's existing default. Point `LOOM_CONFIG_PATH` at your
existing host config directory, not a new empty one. If you previously chose a
different database path, change `storage.sqlite.path` in the Compose inline
config to that existing container-visible path (and persist its mount) **before**
starting. The explicit `--config` now loads `/etc/loom/loom.yaml`, not a previous
`/config/loom.yaml`: transfer your intended kernel settings into the inline
config, keeping credentials outside the public manifest. Neither an image
upgrade nor the setup UI discovers/moves an old database automatically.

## 2. Start and complete setup privately

```bash
docker compose -f deploy/docker/compose.yaml config --quiet
docker compose -f deploy/docker/compose.yaml pull
docker compose -f deploy/docker/compose.yaml up -d
docker compose -f deploy/docker/compose.yaml ps
docker compose -f deploy/docker/compose.yaml logs --tail=100 loom
curl --fail http://127.0.0.1:1925/readyz
```

Open <http://127.0.0.1:1925> and create your administrator account in first-run
setup. **There is no preset admin password.** Keep access private until setup is
complete; the first caller can initialize the administrator. For a remote host,
use `ssh -L 1925:127.0.0.1:1925 user@host` and open the local URL.
Admin setup is saved in `/config/loom.json`; the generated session secret is
persisted in SQLite. Protect and back up both.

The default port binding is loopback. After setup, you may set
`LOOM_BIND_ADDRESS` to a specific LAN address, or `0.0.0.0` with appropriate
firewall rules. Use HTTPS via your existing reverse proxy for remote access,
retain forms authentication, and add `cookie_secure: true` under `auth` in the
inline config when **all browser access uses HTTPS**. Do not enable secure cookies
for the plain HTTP setup connection. Do not publish the service unauthenticated
or enable trusted proxy headers without an explicit proxy trust configuration.

The healthcheck executes `loom healthcheck --url http://127.0.0.1:1925`
directly, probing `/healthz` on the configured port (older release binaries
may have a different healthcheck default); it
does **not** require a shell, curl, or wget in the distroless image. It allows a
five-minute start period; increase it for slow migration/storage. Docker marks
unhealthy containers but does not restart them solely for being unhealthy.
Use `/readyz` to check application/database readiness.

## 3. Connect downloads and media

This deployment does not start a downloader. Configure your existing
[qBittorrent](downloads-qbittorrent.md), [SABnzbd](downloads-sabnzbd.md), or other
[download client](downloads.md) through Loom's UI.

Mount the **same host media root at `/media` in both Loom and the downloader**.
Set the downloader's save/category path to `/media/downloads` (or a subdirectory).
Add Loom libraries under `/media/movies` and `/media/tv`. `localhost` in Loom
means the Loom container, not the downloader or Docker host: use a reachable LAN
hostname, or the service name on a shared Docker network. Protect downloader
credentials and its Web UI too.

The inline YAML explicitly allows the setup filesystem browser to enumerate
`/media` via `filesystem.allowed_browse_roots`; an empty allowlist disables
browsing. Keep this restricted to mounted library/download trees, not `/` or
`/config` (which contains authentication state). If you change the shared root
to `/data`, change the mount, library/download paths **and** this YAML allowlist.

Existing downloaders that report `/downloads` can be accommodated with an
additional bind of the host download directory at `/downloads` in Loom; match the
reported paths exactly and add `/downloads` to `filesystem.allowed_browse_roots`
if you need to browse it in setup. Prefer a single `/media` mount: separate mounts or
filesystems can prevent hardlinks and atomic renames, requiring copies and
additional space. Rain is not enabled here: the GoReleaser release image has no
`/usr/local/bin/rain`, unlike the source-build Dockerfile.

## 4. Upgrade, backup, stop

Record these environment settings in a private operator-managed environment
file (use `--env-file` on every Compose command), or export them in each shell.
No credentials belong in the public Compose example.

Before an upgrade, stop Loom and back up the **whole host config directory**,
including `loom.json`, the DB and any `-wal`/`-shm` files, using your backup tool.
Stopping provides a consistent filesystem copy. Back up media separately.
Change `LOOM_IMAGE_TAG`, then run `pull` and `up -d` again. Check logs and
`/readyz`. If migrations changed the database, rolling back just the image may
not work: restore the matching stopped-state backup too.

```bash
docker compose -f deploy/docker/compose.yaml stop loom
# Back up "$LOOM_CONFIG_PATH" now, then:
docker compose -f deploy/docker/compose.yaml up -d
docker compose -f deploy/docker/compose.yaml down
```

`down` removes the container/network, not the bind-mounted state. Never delete
the host directories to troubleshoot startup. Permission errors mean the
nonroot identity cannot write the state/media directory; fix permissions on the
host rather than making the container root. The image has no interactive shell.

For cluster hosting, see [Kubernetes](kubernetes.md) and [Flux](flux.md).
