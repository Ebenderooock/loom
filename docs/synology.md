# Synology (DSM) installation

Loom can run via Container Manager, or as a native DSM package built using the
included SynoCommunity/spksrc recipe. **A recipe in this repository is not a
published SynoCommunity package.** Use containers today, or build a matching
`.spk` for manual upload. Check release assets and the SynoCommunity catalog
before assuming a prebuilt `.spk` is available.

## Option A — Direct upload (native `.spk`)

1. Identify your NAS CPU architecture and DSM version using Synology's
   [package architecture reference](https://github.com/SynologyOpenSource/pkgscripts-ng/blob/master/Synology_Arch_DSM7.0.md).
   An `x64` package is not interchangeable with `aarch64` or ARMv7, and DSM 6
   packages must not be installed on DSM 7.
2. Obtain a trusted `.spk` matching both architecture and DSM version. If none is
   published, follow the [spksrc build guide](../packaging/synology/README.md#building-locally)
   on a build host, **not on the NAS**. This produces the uploadable installer.
3. In **Package Center → Manual Install**, select the `.spk`, review its publisher
   and requested permissions, and install. Locally built packages are not
   Synology-signed; do not disable DSM signature checks globally.
4. Start Loom from Package Center and open `http://<nas-ip>:1925`.

The recipe currently targets source release `0.1.1`; a builder must update both
version fields and regenerate source digests for a newer release. Building the
recipe does not publish it to SynoCommunity. Native builds and lifecycle behavior
must be checked on the target DSM model before relying on them for production.

## Option B — SynoCommunity package (once published)

Once Loom is published on [SynoCommunity](https://synocommunity.com/), install it
straight from the DSM **Package Center**:

1. **Package Center → Settings → Package Sources** → add
   `https://packages.synocommunity.com/` (name: `SynoCommunity`).
2. Open the **Community** tab, find **Loom**, and click **Install**.
3. After install, open Loom from its Package Center entry or browse to
   `http://<nas-ip>:1925`.

The package keeps `loom.yaml` and `data/loom.db` under its private
`/var/packages/loom/var` directory (`SYNOPKG_PKGVAR`), preserved across upgrades.

### Media folder permissions

Loom runs as an isolated DSM service user. For it to scan libraries and import
downloads, grant that user **read/write** access to the relevant shared folders:

> Control Panel → Shared Folder → select folder → **Edit → Permissions** → grant
> **System internal user** → grant the Loom package user read/write.

Grant only the package account access, not all users. Native Loom sees NAS paths
such as `/volume1/data/media` and `/volume1/data/downloads`; configure the download
client to report matching paths or configure Loom remote path mappings. Keeping
downloads and media in the same shared folder/filesystem allows hardlinks.

To enable the setup UI's filesystem browser, add the intended shared folder
to the package's `loom.yaml` and restart Loom:

```yaml
filesystem:
  allowed_browse_roots:
    - /volume1/data
```

### TLS

Loom serves plain HTTP. For HTTPS, put it behind DSM's built-in reverse proxy:

> Control Panel → Login Portal → **Advanced → Reverse Proxy** → forward an HTTPS
> hostname to `localhost:1925`.

The packaging recipe (for contributors) lives in
[`packaging/synology/`](../packaging/synology/) and can be submitted to
[`SynoCommunity/spksrc`](https://github.com/SynoCommunity/spksrc).

## Option C — Docker (Container Manager)

Requires a model supported by Container Manager (or Docker on older DSM).
The published image supports **amd64 and arm64**, not 32-bit ARM NAS models.
Use **Container Manager → Project → Create** and paste this Compose file:

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
      - /volume1/data:/data
    restart: unless-stopped
    stop_grace_period: 60s
```

Create `/volume1/docker/loom` and `/volume1/data/{media,downloads}` first. The
image runs as UID/GID **65532:65532**. Grant that account read/write access to the
config directory and only the required data directories using DSM permissions
or SSH ownership/ACL tools. Do not use world-writable permissions. Unlike
linuxserver images, Loom does not interpret `PUID`/`PGID`. To use another account,
add `user: "UID:GID"` to the Compose service, replacing both with its numeric
IDs from `id`, and grant that account access to config and data. See the
[Docker guide](docker.md) for storage and shared-path details.

For the container filesystem browser, use the same YAML block in
`/volume1/docker/loom/loom.yaml`, but replace `/volume1/data` with `/data`
(the path seen inside the container) before starting the project.

Map `/volume1/data:/data` into the download client too, so it reports
`/data/downloads` and Loom uses `/data/media`. The `/config` SQLite database should
stay on local NAS storage, not an SMB/NFS mount. Replace `latest` with a tested
release tag without the `v` prefix (for example `0.1.1`) or image digest for
repeatable upgrades.

## Upgrade, backup, and removal

Stop Loom before copying its state directory, including `data/loom.db`, any SQLite
`-wal`/`-shm` files, and `loom.yaml`. Keep the backup outside the package directory.
For native installations, use Manual Install with the newer matching `.spk`, or
Package Center Update once published. For containers, pull the chosen image and
recreate the project without deleting its bind-mounted folders.
If an existing project uses `/media` or a custom database path, retain those
mounts and settings during the upgrade; the `/data` layout above is for new
installations, not an automatic library-path migration.

Database migrations can make a downgrade incompatible: roll back using the old
binary/image **and the pre-upgrade state backup**, not just an older image.
Verify the UI and `http://<nas-ip>:1925/readyz` after upgrading. Uninstalling a
DSM package may delete its private state; back it up before removal. Removing a
Container Manager project must not include deleting the host data directories
unless you deliberately intend to erase them.
