# Loom — SynoCommunity (spksrc) packaging

This directory holds the [spksrc](https://github.com/SynoCommunity/spksrc) recipe
that builds Loom as a native Synology DSM package (`.spk`) installable from the
**Package Center**.

```
cross/loom/Makefile            # cross-compile recipe (native/go, CGO_ENABLED=0, -tags embed)
spk/loom/Makefile              # package metadata, ports, dependencies
spk/loom/src/service-setup.sh  # DSM service wiring (loom serve)
spk/loom/src/loom.sc           # firewall port definition (FWPORTS)
spk/loom/src/loom.png          # 256x256 package icon (SPK_ICON)
```

## Why this is feasible

Loom is a single static binary built with `CGO_ENABLED=0` using the pure-Go
`modernc.org/sqlite` driver, so supported Go/SQLite targets need no C toolchain
— the same model as the pure-Go `syncthing` package. Not every DSM architecture
is supported; validate the target rather than assuming universal support. The React web
UI is compiled into the binary via the `embed` build tag.

## How the source is consumed

The web UI assets (`web/dist/`) are **gitignored**, so GitHub's auto-generated
"Source code" tarball cannot be compiled with `-tags embed`. Instead, each tagged
Loom release publishes a **curated source tarball** asset:

```
loom-source-v<version>.tar.gz
```

produced by `.github/workflows/release.yml`. It contains the tracked sources plus
the prebuilt `web/dist/`, so spksrc can run `go build -tags embed` with **no Node
toolchain**. `cross/loom/Makefile` points `PKG_DIST_SITE` at this asset.

> Prerequisite: a stable, semver-tagged Loom release (e.g. `v0.1.0`) must exist
> before the package can be built. Bump `PKG_VERS`/`SPK_VERS` to match the tag.

## Building locally

Use the official [spksrc build environment](https://github.com/SynoCommunity/spksrc#setup-development-environment)
on a Linux amd64 host, or its Linux amd64 Docker environment on macOS
(Apple Silicon requires emulation). Windows is not supported by that development
environment. Building on the NAS itself is not the intended path.

Choose a Loom release with a curated source asset. Update `PKG_VERS` in
`cross/loom/Makefile` and `SPK_VERS` in `spk/loom/Makefile` to that same version
before generating digests; increment `SPK_REV` for packaging-only changes.
The checked-in recipe and digests currently target `0.1.1`.

```sh
git clone https://github.com/SynoCommunity/spksrc
cd spksrc
cp -r /path/to/loom/packaging/synology/cross/loom cross/loom
cp -r /path/to/loom/packaging/synology/spk/loom    spk/loom

# Enter the official spksrc environment; on macOS also add
# -e TAR_CMD="fakeroot tar" as described in the upstream setup guide.
docker run --rm -it --platform=linux/amd64 \
  -v "$PWD:/spksrc" -w /spksrc ghcr.io/synocommunity/spksrc /bin/bash

# Inside the container, generate digests after the release asset exists:
make -C cross/loom digests
cat cross/loom/digests   # commit this in the spksrc PR

# Build for a representative arch (e.g. x86_64 / aarch64):
make -C spk/loom arch-x64-7.1
make -C spk/loom arch-aarch64-7.1

# For contributor matrix validation (not a promise of support):
make -C spk/loom all-supported
```

The resulting `.spk` files land in the spksrc checkout's `packages/` directory.
Match the package's architecture and DSM version to the NAS, then install via
**Package Center → Manual Install**. Retain the build's spksrc commit, build image
digest, Loom source digest, and target for reproducibility. These packages are
locally built, not signed or published by SynoCommunity.

## DSM runtime notes

- **State directory:** all config + the sqlite DB live under `${SYNOPKG_PKGVAR}`
  (`LOOM_CONFIG_DIR` and `LOOM_DATA_DIR`; the default database path is
  `${SYNOPKG_PKGVAR}/data/loom.db`). A custom `storage.sqlite.path` in `loom.yaml`
  remains honored. DSM preserves state across upgrades; back it up separately
  before uninstalling.
- **Port:** Loom serves HTTP on `SERVICE_PORT` (default `1925`). Put it behind the
  DSM reverse proxy (Control Panel → Login Portal → Advanced → Reverse Proxy) for
  TLS — Loom does not terminate TLS itself.
- **Media access (important):** the package runs as an isolated DSM service user
  (`SERVICE_USER = auto`). For Loom to scan/import media and write to download
  folders, grant that user **read/write** on the relevant shared folders
  (Control Panel → Shared Folder → Edit → Permissions), or run your download
  client's completed folder where the Loom user has access.

## Submitting upstream

1. Open a PR to `SynoCommunity/spksrc` adding `cross/loom` and `spk/loom` (with the
   generated `cross/loom/digests`).
2. Pass CI (`all-supported` build) and address maintainer review.
3. Finalize `UNSUPPORTED_ARCHS` empirically from the build matrix results.
4. Once merged and published, update Loom's docs to link the Package Center entry.
