# Synology (DSM) installation

Loom can run on a Synology NAS as a native DSM package built from the
[packaging recipe](../packaging/synology/), or via Docker (Container Manager).
GitHub releases provide source and binaries, not ready-made `.spk` packages.

## Option A — Native DSM package (`.spk`)

### Manual installation

1. Build or obtain a `.spk` for your NAS architecture and DSM version. For a
   **DS1817+ running DSM 7.3.1**, use the **x64 / DSM 7.1** package
   (`loom_x64-7.1_<version>-<revision>.spk`); it installs on DSM 7.1 and later. The
   spksrc `x64-7.3` target requires DSM 7.3-86009 or later, so DSM 7.3.1
   (build 86003) rejects it.
2. In **Package Center → Manual Install**, select the `.spk`. A local build is
   unsigned; review the package source before accepting any installation warning.
3. Open Loom from its Package Center entry or browse to `http://<nas-ip>:1925`.
   Complete initial account setup before exposing the service to other networks.

The package runs as the non-root **`sc-loom`** system internal user. Its writable
state lives in `/var/packages/loom/var` (`SYNOPKG_PKGVAR`), which DSM preserves
across package upgrades:

| Location | Contents |
|---|---|
| `loom.json` | Application settings |
| `loom.yaml` | Optional server configuration, including folder-picker roots |
| `data/loom.db` | SQLite database and its WAL/SHM files |
| `backups/` | Backups, created on demand |
| `cache/` and `tmp/` | Package-local cache and temporary directories |
| `loom.log` | Service stdout/stderr |

`LOOM_CONFIG_DIR`, `LOOM_DATA_DIR`, and `HOME` point at this writable directory.
`XDG_CACHE_HOME` and `TMPDIR` point at its cache and temporary subdirectories.
Startup checks directory write/search permissions and reports failures instead
of trying to store application state in the read-only installation directory.
Do not delete or relocate the database independently of its configuration.

### Media folder permissions

The install and upgrade wizard asks for two shared folders: **media** (your
library, default `media`) and **downloads** (completed downloads, default
`downloads`). DSM creates a share if it is missing and automatically grants the
package user **`sc-loom`** Read/Write on both. Upgrading from an earlier package
shows the same page, prefilled with the previous choices, so existing installs
get access without manual steps. Use two different shared folders.

Loom also offers the granted shares as folder-picker roots (for example
`/volume1/media` and `/volume1/downloads`). Folders outside them are refused.
To choose the roots yourself, set `filesystem.allowed_browse_roots` in
`/var/packages/loom/var/loom.yaml` and restart the package. That setting
overrides the automatic roots.

Other shared folders, such as a separate recycle bin, still need manual access:
open **Control Panel → Shared Folder → select folder → Edit → Permissions**,
select **System internal user**, find **`sc-loom`**, and grant **Read/Write**.
Make sure no explicit deny on subfolders blocks the package user.

Grant the download client's account equivalent access to the shared download
folder. Loom uses a group-writable file creation mask (`0002`) for cooperation
with clients in the same group; DSM ACLs and existing file permissions still
apply. Moving or cleaning completed downloads also requires permission to
delete from their source directory. Do not grant the package root access or
make shares world-writable.

### BitTorrent clients on or outside the NAS

The native package supports external **qBittorrent, Transmission, and Deluge**
clients. Add the client in Loom's download-client settings with its hostname/IP,
Web UI or RPC port, credentials, and HTTPS setting. Use the built-in connection
test before starting a download.

| Client location | Connection address |
|---|---|
| Native client on this NAS | `127.0.0.1` and its configured API port |
| Container Manager client on this NAS | NAS address and the container's published host port |
| Another LAN device | That device's hostname/IP and API port |
| Outside the LAN | A reachable VPN address, or a trusted HTTPS endpoint |

Use a hostname/IP in the host field, not a full URL. Reverse-proxy subpaths can
be supplied through the client's advanced configuration (`base_path` for
qBittorrent/Deluge, `rpc_url` for Transmission). Ensure the client's listener,
authentication, DSM firewall, routing and any client VPN allow the connection.
HTTPS requires a certificate trusted by the NAS; do not expose an unauthenticated
client API to the internet.

For **Deluge**, Loom talks to the Web UI (default port `8112`, password
`deluge` until changed), not the daemon port `58846`. Open the Deluge Web UI
once, choose the daemon in **Connection Manager**, and click **Connect**. If
the connection test later reports that the Web UI is not connected to a daemon
(for example after Deluge restarts), reconnect it in Connection Manager.

Loom shares one status request per external client across every open browser
tab and background job. It refreshes queue state at most every 5 seconds and
immediately after you add, pause, resume or remove an item. When a client fails,
Loom waits before retrying, starting at 10 seconds and doubling up to 5 minutes.
It also waits after rejected logins. The **Downloads** page names each
unreachable client, shows its error and the time until the next retry. A
successful connection test clears the wait immediately. The built-in torrent
engine runs inside Loom and is always read live.

**Connecting to a remote client does not transfer its downloaded files.** Its
completed-download directory must also be mounted or synchronized onto the NAS
and readable by `sc-loom`. Configure a per-client remote path mapping when paths
differ: for example, `/downloads` reported by the client maps to
`/volume1/downloads` on the NAS. Path mappings translate paths only. Hardlinks
require the source and library to share a filesystem; normal hardlink mode can
fall back to copying, while `hardlink_only` cannot.

The separate built-in torrent option requires a Rain sidecar; this native
package does not include one. External clients do not require Rain.

### Verify on the NAS

After installation and permission setup, check that settings survive a package
restart, create a database backup, and import a small completed download into
each configured library. Confirm the download client can still access files
after import, and check `loom.log` for permission or connection errors. Local
package checks cannot establish your NAS's ACLs, firewall rules or client
credentials.

On the Movies and Series pages, **Import Existing** scans a configured library
and matches its contents. The dialog shows **Starting scan** while the request
is pending, then progress or the server's error. If it reports a missing path
or permission error, check the library path and `sc-loom`'s access to the share;
do not grant access to unrelated folders. Background data refreshes do not clear an
active scan's progress or its completed results.

### TLS for Loom

Loom serves plain HTTP. For HTTPS, put it behind DSM's built-in reverse proxy:

> Control Panel → Login Portal → **Advanced → Reverse Proxy** → forward an HTTPS
> hostname to `localhost:1925`.

### SynoCommunity repository

If Loom is listed on [SynoCommunity](https://synocommunity.com/) for your NAS, it
can also be installed from the **Community** tab after adding
`https://packages.synocommunity.com/` to Package Center's package sources.
Publishing to that repository is separate from Loom's GitHub release workflow.

## Option B — Docker (Container Manager)

If you prefer Docker, use the published image with Container Manager:

```yaml
services:
  loom:
    image: ghcr.io/ebenderooock/loom:latest
    container_name: loom
    ports:
      - "1925:1925"
    environment:
      LOOM_CONFIG_DIR: /config
      LOOM_DATA_DIR: /config
    volumes:
      - /volume1/docker/loom:/config
      - /volume1/media:/media
    restart: unless-stopped
```

Map your media/download shared folders into the container and ensure the
container user can read/write them.
