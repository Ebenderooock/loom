# Raspberry Pi hosting

Two supported pathways are a native Linux service or a container. Prefer
**64-bit Raspberry Pi OS Lite** on a Pi with an ARMv8 CPU (for example Pi 3,
4, 5 or Zero 2 W). OS bitness matters as much as CPU capability.

## Select the right build

```sh
uname -m
getconf LONG_BIT
```

| CPU and installed OS | Loom pathway |
|---|---|
| ARMv8 CPU + 64-bit Linux (`aarch64`, 64) | Native `linux_arm64`, or arm64 container |
| ARMv7-capable CPU + 32-bit Linux | Native `linux_armv7` only |
| Original Pi 1 / Zero / Zero W (ARMv6) | **Unsupported** |

Some 32-bit Pi kernels report `armv7l`/`armv8l`, and some 64-bit kernels run
32-bit userlands. Check `getconf LONG_BIT` and your OS documentation, not just
the model label or kernel string. A Pi 2 revision with ARMv7 can use the native
ARMv7 build; an ARMv8 Pi running 32-bit userspace also needs that build.
There is **no ARMv6 binary** and no Windows/macOS Pi installation path here.
Do not infer ARMv6 support from the existence of a Go ARMv6 toolchain.

## Native service (recommended for 32-bit ARMv7)

Use [the Linux systemd installer guide](linux.md), selecting the corresponding
Linux archive from [Loom releases](https://github.com/Ebenderooock/loom/releases)
and verifying its SHA-256. The archive must actually exist for that release;
these instructions do not promise unpublished assets. From the extracted root:

```sh
chmod +x ./loom
sudo sh packaging/linux/install.sh ./loom
systemctl status loom --no-pager
journalctl -u loom -n 100 --no-pager
```

The service runs as `loom:loom`; config is `/etc/loom`, data/definitions are
`/var/lib/loom`, and SQLite is **`/var/lib/loom/loom.db`**. For first-run setup
from another computer, keep the installer default loopback address and tunnel:

```sh
ssh -L 1925:127.0.0.1:1925 YOUR_USER@YOUR_PI
```

Open http://127.0.0.1:1925 on that computer. Complete setup before allowing
LAN access; follow the Linux guide for listen address, firewall, media group
permissions, mount dependencies, logs, upgrades and data-preserving uninstall.
Use a stable DHCP reservation/static address for download-client integrations.

## Container (64-bit only on Pi)

Loom's published container targets are **linux/amd64 and linux/arm64 only**.
There is no arm/v7 or ARMv6 image. Do not try to use an arm64 image on a
32-bit OS, or rely on emulation for a media automation service.

Install Docker Engine and the Compose plugin using your OS's supported
instructions. Select an actual published versioned image tag and inspect it:

```sh
docker buildx imagetools inspect ghcr.io/ebenderooock/loom:YOUR_VERSION
```

Confirm `linux/arm64` is present. Follow [the Docker guide](docker.md) for a
Compose deployment with persistent config/data and matching library/download
mounts. Pin the chosen version instead of relying on `latest` for unattended
upgrades. Native installation and Docker are alternatives: do not point both
at the same SQLite database or run both on the same port.

## Storage, reliability and source builds

- Prefer reliable SSD/USB storage for database, downloads and media; heavy
  downloads and SQLite writes wear microSD cards. Retain backups off the Pi.
- If moving the native database/data to an SSD, stop Loom, copy the stopped
  data (including sidecars), adjust **both `data_dir` and `storage.sqlite.path`**
  to absolute paths in `/etc/loom/loom.yaml`, grant loom access, and add
  `RequiresMountsFor=` to the unit. Do not move only the YAML file and assume
  other paths follow it.
- Ensure NAS/USB mounts exist before imports, and use identical path mappings
  between Loom and download clients. Never solve permissions with `chmod 777`.
- Limit concurrent downloads/search work for smaller models, monitor memory
  and disk usage, and keep cooling/power adequate. No throughput promise is
  implied for a particular Pi model.
- If no suitable archive has been published, build on a workstation with the
  repository's required Go/Node versions and a prebuilt frontend (`make
  web-build`). With frontend assets present, cross-compile:

```sh
# 64-bit Pi
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags embed -o loom-linux-arm64 ./cmd/loom
# 32-bit ARMv7 Pi
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags embed -o loom-linux-armv7 ./cmd/loom
```

Copy the matching binary plus `packaging/linux/` to the Pi. Pass that binary's
path explicitly to `install.sh`. An API-only build without `embed` does not
provide the production web UI. ARMv6 remains outside the supported target set.
