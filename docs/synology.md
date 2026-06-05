# Synology (DSM) installation

Loom can run on a Synology NAS in two ways: as a native DSM package (when the
SynoCommunity package is published) or via Docker (Container Manager) today.

## Option A — SynoCommunity package (native `.spk`)

Once Loom is published on [SynoCommunity](https://synocommunity.com/), install it
straight from the DSM **Package Center**:

1. **Package Center → Settings → Package Sources** → add
   `https://packages.synocommunity.com/` (name: `SynoCommunity`).
2. Open the **Community** tab, find **Loom**, and click **Install**.
3. After install, open Loom from its Package Center entry or browse to
   `http://<nas-ip>:8989`.

The package keeps all state (config + sqlite database) under the package's
private `var` directory, which DSM preserves across upgrades.

### Media folder permissions

Loom runs as an isolated DSM service user. For it to scan libraries and import
downloads, grant that user **read/write** access to the relevant shared folders:

> Control Panel → Shared Folder → select folder → **Edit → Permissions** → grant
> the Loom package user (or the `users` group) read/write.

### TLS

Loom serves plain HTTP. For HTTPS, put it behind DSM's built-in reverse proxy:

> Control Panel → Login Portal → **Advanced → Reverse Proxy** → forward an HTTPS
> hostname to `localhost:8989`.

The packaging recipe (for contributors) lives in
[`packaging/synology/`](../packaging/synology/) and is submitted to
[`SynoCommunity/spksrc`](https://github.com/SynoCommunity/spksrc).

## Option B — Docker (Container Manager)

If you prefer Docker, use the published image with Container Manager:

```yaml
services:
  loom:
    image: ghcr.io/ebenderooock/loom:latest
    container_name: loom
    ports:
      - "8989:8989"
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
