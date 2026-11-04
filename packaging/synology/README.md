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

> Prerequisite: a stable, semver-tagged Loom release (currently `v0.4.1`) must exist
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
cp -r /path/to/loom/packaging/synology/cross/loom/. cross/loom/
cp -r /path/to/loom/packaging/synology/spk/loom/.    spk/loom/
```

Use a Linux/amd64 build environment with GNU make 4.3 or later (spksrc uses
grouped targets; older builder images fail with "mixed implicit and normal
rules"). An `ubuntu:24.04` container with spksrc's build packages works. On
macOS, run it with `--platform linux/amd64` and `-e TAR_CMD="fakeroot tar"` so
that the package records root-owned files rather than the macOS account's UID.
Inside that environment:

```sh
make setup

# Enter the official spksrc environment; on macOS also add
# -e TAR_CMD="fakeroot tar" as described in the upstream setup guide.
docker run --rm -it --platform=linux/amd64 \
  -v "$PWD:/spksrc" -w /spksrc ghcr.io/synocommunity/spksrc /bin/bash

# Inside the container, generate digests after the release asset exists:
make -C cross/loom digests
cat cross/loom/digests   # commit this in the spksrc PR

# x86-64 models such as the DS1817+ (Avoton) on DSM 7.1–7.3.x. Do not use
# arch-x64-7.3 for DSM 7.3.1: it sets os_min_ver=7.3-86009 (> 7.3.1-86003).
make -C spk/loom arch-x64-7.1

# Other representative arch (aarch64):
make -C spk/loom arch-aarch64-7.1

# For contributor matrix validation (not a promise of support):
make -C spk/loom all-supported
```

Use a fresh spksrc checkout for a new build, or clean the existing Loom build
before updating the recipe. spksrc already contains an older Loom recipe:
copy the directory **contents** as above, not a nested `loom/loom` directory,
and verify the resulting package version rather than reusing old build cookies.

The resulting `.spk` files land in spksrc's top-level `packages/` directory.
For the DSM 7.1+ x64 target, the filename is `loom_x64-7.1_0.4.1-2.spk`.
Install it on DSM via **Package Center → Manual Install**. GitHub Actions
currently publishes the source tarball, not these `.spk` files.

Run the package service setup checks from the Loom repository as a non-root
user (no NAS or spksrc checkout required):

```sh
sh packaging/synology/tests/service-setup.sh
```

DSM sources service setup during installation before package paths are available.
Runtime path validation and environment changes must remain in `service_prestart`,
not at file scope, so that fresh installations and upgrades can run `preinst`.

## DSM runtime notes

- **State directory:** config and `data/loom.db` live under `${SYNOPKG_PKGVAR}`
  (`LOOM_CONFIG_DIR`, `LOOM_DATA_DIR`, and `HOME`). `XDG_CACHE_HOME` and `TMPDIR`
  use writable `cache/` and `tmp/` subdirectories. Startup fails with a clear
  error if its state directories are not writable. DSM preserves the package
  data directory across upgrades; back it up separately before uninstalling.
  Custom `storage.sqlite.path` in `loom.yaml` remains honored.
- **Build output:** `.spk` files land in spksrc's `packages/` directory. Match the
  package architecture and DSM version to the NAS. Retain the spksrc commit,
  build image digest, Loom source digest and target for reproducibility. Local
  packages are not signed or published by SynoCommunity.
- **Port:** Loom serves HTTP on `SERVICE_PORT` (default `1925`). Put it behind the
  DSM reverse proxy (Control Panel → Login Portal → Advanced → Reverse Proxy) for
  TLS — Loom does not terminate TLS itself.
- **Media access:** `SERVICE_USER = auto` creates **`sc-loom`**. The
  install/upgrade wizard (`src/wizard/`) asks for a media and a downloads shared
  folder; the `data-share` resource (`SERVICE_WIZARD_SHARENAME` plus the
  `loom_extra_shares` hook) makes DSM grant `sc-loom` Read/Write on both. The
  choices are saved to `${SYNOPKG_PKGVAR}/synology-shares.conf` to prefill the
  upgrade wizard. Other shares still need manual permissions. The package does
  not recursively change ownership. A `0002` creation mask allows shared-group writes.
- **Folder picker:** at startup the granted shares under
  `/var/packages/loom/shares` become `LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS`
  unless `filesystem.allowed_browse_roots` is set in `${SYNOPKG_PKGVAR}/loom.yaml`.
- **Download clients:** qBittorrent, Transmission and Deluge may run on this NAS
  or a reachable remote host. Remote API access still requires locally accessible
  completed files and path mappings when paths differ. No Rain sidecar is
  included. See [the installation guide](../../docs/synology.md) for permissions,
  client addresses and NAS verification steps.

## Submitting upstream

1. Open a PR to `SynoCommunity/spksrc` updating `cross/loom` and `spk/loom` (with the
   generated `cross/loom/digests`).
2. Pass CI (`all-supported` build) and address maintainer review.
3. Finalize `UNSUPPORTED_ARCHS` empirically from the build matrix results.
4. Once merged and published, update Loom's docs to link the Package Center entry.
