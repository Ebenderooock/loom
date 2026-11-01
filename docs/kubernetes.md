# Run Loom on Kubernetes

Loom already has a Helm chart at
[`deploy/helm/loom`](../deploy/helm/loom). Install from a checkout of this
repository; this guide does **not** assume a published OCI Helm chart or Helm
repository. For GitOps reconciliation of the same chart, see [Flux](flux.md).
For a single Docker host, see [Docker](docker.md).

## Prerequisites and storage decisions

- A working Kubernetes cluster and `kubectl` configured for it.
- [Helm](https://helm.sh/docs/intro/install/) and access to GHCR from cluster nodes.
- A provisioner/StorageClass for persistent state, or a prepared PVC in the
  installation namespace.
- Optional existing media/download PVCs and a reachable external downloader.
  Ingress, TLS and NAS provisioning are cluster-specific, not installed by Loom.

The default database is SQLite at **`/config/data/loom.db`**. The chart mounts
`/config` persistently, runs one replica, and uses **Recreate** upgrades so two
Loom processes do not write the same database concurrently. The chart rejects
other replica counts, including when a different storage engine is configured;
it is not an HA or horizontally scalable deployment.

Select **local or block-backed storage with correct SQLite file locking**, not
NFS/SMB for `/config`. `ReadWriteOnce` alone does not prove locking safety, nor
does it prevent multiple pods on one node. A local PV pins data to its node:
node failure means downtime until that node/storage is recovered. Prefer a
provisioner that sets PV node affinity (often `WaitForFirstConsumer` binding).
If you use `hostPath` instead, you must handle permissions and node placement
yourself; identical paths on different nodes are not shared data. Back up local
state off-node. Block storage can move to another node only as supported by its
CSI driver; Recreate upgrades may wait for detach/attach.

NAS/NFS/SMB is suitable for **media and download content**, via a PVC provisioned
by your existing cluster storage integration. Mount one shared root containing
`downloads`, `movies`, `tv`, etc. at `/media` in Loom and the downloader when
possible. Hardlinks require the same filesystem; separate PVCs/mounts may force
copies. Create those directories and grant permissions before importing.

## 1. Configure your release

Choose a stable [Loom release](https://github.com/ebenderooock/loom/releases) and
check out its Git tag **containing the chart changes you intend to deploy**.
Container tags are **`X.Y.Z` without `v`**, while Git release tags are `vX.Y.Z`.
The chart's default appVersion is not a recommendation for the newest image;
explicitly choose a published image version.

Create a private operator values file, e.g. `loom-values.yaml`, replacing both
placeholders:

```yaml
image:
  tag: X.Y.Z
rain:
  enabled: false
persistence:
  config:
    enabled: true
    storageClass: YOUR_SQLITE_SAFE_STORAGE_CLASS
    size: 1Gi
  media:
    enabled: false  # enable with a prepared PVC before importing content
service:
  type: ClusterIP
ingress:
  enabled: false
```

An empty `storageClass` uses the cluster default. Do not leave it empty unless
you have verified that default is SQLite-safe. To reuse state, set
`persistence.config.existingClaim: loom-config-existing` to a PVC in the same
namespace; the chart will not create a new config claim. Back up old state before
migrating: `storage.sqlite.path` belongs to kernel YAML, not `loom.json`'s
`persistence.path`. Match the existing database path to avoid accidentally
initializing a fresh database.

The chart renders `config` values into a read-only `/etc/loom/loom.yaml`, used
with `serve --config`. Defaults include:

```yaml
config:
  config_dir: /config
  data_dir: /config
  storage:
    engine: sqlite
    sqlite:
      path: /config/data/loom.db
  auth:
    mode: forms
  filesystem:
    allowed_browse_roots:
      - /media
```

Keep the config/data directories at `/config` unless you also supply matching
writable volume mounts. Keep the database on that persisted volume. Changing
the rendered config triggers a pod restart via its checksum. UI-managed setup
and libraries remain in writable `/config/loom.json`. Do not put credentials,
admin password hashes or session secrets in a ConfigMap/public values file.
The application creates and persists its session secret in SQLite automatically.
For optional secret environment settings, use `extraEnv` with Kubernetes
`valueFrom.secretKeyRef` and an independently managed Secret; do not embed
credentials in Helm values.

### Media and downloads

Add this to your values **after preparing `shared-media` in the same namespace**:

```yaml
persistence:
  media:
    enabled: true
    existingClaim: shared-media
    mountPath: /media
```

Merge it into the existing `persistence` section; do not duplicate YAML keys.
Use the same claim and `/media` mount in the downloader, with its save path
`/media/downloads`. Configure libraries at `/media/movies` and `/media/tv`.
If the downloader runs on a different node, the media claim must support that
access pattern (typically RWX for a NAS). PVC access modes are only used when
the chart creates a claim; they do not alter an existing claim.

For a downloader that reports `/downloads` on a separate share, also configure:

```yaml
persistence:
  downloads:
    enabled: true
    existingClaim: shared-downloads
    mountPath: /downloads
config:
  filesystem:
    allowed_browse_roots:
      - /media
      - /downloads
```

The chart explicitly allows `/media` in the setup filesystem browser.
An empty `filesystem.allowed_browse_roots` disables browsing; update this YAML
allowlist as well as mounts and library/download paths when using a custom root
such as `/data`. Restrict it to library/download trees, not `/` or `/config`.
Without a media mount, initial admin setup works but `/media` has no library
content to browse; attach storage before configuring/importing libraries.

Alternatively, use `extraVolumes` and `extraVolumeMounts` for existing library
PVCs at custom paths. Do not reuse chart volume names (`config`, `runtime-config`,
`tmp`, `media`, `downloads`, `rain-config`) or mount over `/config` or `/etc/loom`.
When enabling optional Rain, set `rain.downloadDir` to a writable mounted path
(e.g. `/downloads`); the path is shared by Loom and Rain. Changing mount paths
does not automatically change library paths or the external downloader's settings.
Use a downloader Service DNS name such as `qbittorrent.media.svc.cluster.local`,
not `localhost` (which refers to the Loom pod).

### Nonroot permissions

Defaults run as UID/GID **65532**, `fsGroup: 65532`, with no added capabilities,
no privilege escalation, read-only root filesystem and no service account token.
`/tmp` is a bounded writable `emptyDir`. If an existing media share uses a
different group, set `podSecurityContext.fsGroup` and, if needed,
`securityContext.runAsGroup` to that group; keep `runAsNonRoot: true`.

CSI support for `fsGroup` varies. NFS root-squash/SMB ACLs may prevent Kubernetes
from changing ownership. Pre-grant the Loom identity/group read/write/traverse
access on the NAS and verify the CSI mount options. Changing `fsGroup` is not a
replacement for NAS ACL configuration. Do not run Loom as root or add a
privileged chown init container as a shortcut.

## 2. Validate and install

From the checked-out repository:

```bash
helm lint ./deploy/helm/loom -f loom-values.yaml
helm template loom ./deploy/helm/loom -n media -f loom-values.yaml
helm upgrade --install loom ./deploy/helm/loom \
  --namespace media --create-namespace -f loom-values.yaml \
  --wait --timeout 15m
kubectl -n media get pods,pvc,svc
kubectl -n media logs deployment/loom -c loom --tail=100
kubectl -n media port-forward svc/loom 1925:1925
```

Open <http://127.0.0.1:1925>, create the administrator account, then configure
libraries and download clients. **No preset credentials exist.** Keep the
ClusterIP private and leave ingress disabled until initialization is complete;
the first caller can initialize the account. Port-forward listens on loopback
by default. In another terminal, check:

```bash
curl --fail http://127.0.0.1:1925/readyz
```

Probes use unauthenticated `/healthz` (startup), `/livez` (liveness), and
`/readyz` (readiness, including a database ping). Startup allows approximately
**10 minutes** (`60 × 10s`); the Deployment progress deadline is **15 minutes**.
For larger migrations/slow storage, increase `probes.startup.failureThreshold`,
`progressDeadlineSeconds`, and Helm/Flux timeouts together, leaving room for
volume attachment and readiness. Do not use an aggressive liveness probe to
interrupt database migration.

For LAN/remote browser access, configure your existing ingress controller and
TLS Secret through `ingress.className`, `ingress.hosts`, and `ingress.tls`.
Set `config.auth.cookie_secure: true` when browser access is exclusively HTTPS.
Retain authentication and enforce network/firewall policy appropriate to your
cluster. Loom installs no ingress controller, certificate operator, or storage
operator. `deploy/helm/values-homelab.yaml` is a site-specific example, **not**
a portable turnkey installation: adjust its image, storage class, existing
claims, identity, hostname and TLS Secret before using it.

### Optional Rain sidecar

Rain is **disabled by default** because the GHCR GoReleaser image does not ship
`/usr/local/bin/rain`. Use external download clients for the minimal deployment.
If you want Rain, build a pinned image from `deploy/docker/Dockerfile` (which
includes Rain), make it available to the cluster, set `rain.enabled: true` and
set `rain.image.repository`/`rain.image.tag` to that image. The chart rejects an
enabled sidecar without an explicit image. Prepare a writable `rain.stateDir`
(default `/config/rain`) on the config volume and a shared `rain.downloadDir`.
The RPC binds to loopback and is not exposed by the Service. No packages are
installed at pod startup. Verify the optional image and sidecar separately;
Loom's probes do not certify Rain health.

## 3. Operate, back up, and upgrade

- Pending PVC/pod: inspect `kubectl -n media describe pvc` and
  `kubectl -n media describe pod POD_NAME` for provisioning, node affinity or
  multi-attach failures. Check storage class and media claim existence.
- CrashLoopBackOff: check `logs ... --previous`, DB path, permissions and config.
  Distroless has no shell; do not expect `kubectl exec ... -- sh` to work.
- Not ready: inspect `/readyz` and logs; `/livez` alone does not prove DB access.
  Check storage availability before increasing deadlines.

Before upgrades, obtain a consistent backup of **all `/config` state**, including
`loom.json`, SQLite and any WAL/SHM files. For a filesystem copy, stop Loom during
backup: scale the Deployment to zero, wait for its pod to terminate, then use
your storage backup mechanism. Local-volume backups must run where that volume
is accessible. Restore the replica count to one afterward. Do not copy just the
live `loom.db`. Protect backups: they contain credentials/auth material. Back up
media separately.

Change `image.tag` and use the validated chart revision with `helm upgrade`,
`--wait --timeout 15m`. Recreate means brief downtime. If migrations are not
backward-compatible, `helm rollback` alone cannot restore data; restore the
matching backup as well.

**Upgrading from the older chart:** the kernel config is now explicitly loaded
from `/etc/loom/loom.yaml`, not implicitly from `/config/loom.yaml`. Transfer
your nonsecret kernel settings into `config` values and secret settings into
Secret-backed environment variables before upgrading. Preserve your existing
SQLite path. The default UID/GID changed from `999:1001` to the image's
`65532:65532`; retain your old nonroot identity via both security contexts or
prepare state-file permissions while stopped (especially `loom.json`, mode
0600). Rain is now opt-in with its own explicitly selected executable-bearing
image; remove obsolete `torrent` values if present, since they still trigger
legacy sidecar enablement.

Chart-created PVCs are ordinary Helm resources: **uninstalling the release can
delete them** and the PV reclaim policy may then delete data. Back up first.
For independently managed lifecycle/retention, supply `existingClaim` and manage
the PVC/PV outside the release. Never delete claims just to fix a stuck pod.
