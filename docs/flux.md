# Run Loom with Flux

[Flux](https://fluxcd.io/flux/) keeps Kubernetes aligned with your GitOps
repository. The minimal [`deploy/flux`](../deploy/flux) example contains a
Namespace, a pinned `GitRepository`, and a `HelmRelease` that builds Loom's chart
from **`./deploy/helm/loom` relative to the Loom source repository root**.
It does not require an OCI Helm chart publication, image automation, or another
operator.

Read [Kubernetes](kubernetes.md) first for SQLite-safe storage, permissions,
media/download mounts, first-run authentication, and backup requirements.
This example uses a private ClusterIP, one replica, persisted SQLite, and an
external downloader; media imports require you to attach your media storage.

## Short setup guide

### 1. Bootstrap your cluster once

Install the [Flux CLI](https://fluxcd.io/flux/installation/) and use its
[bootstrap guide](https://fluxcd.io/flux/installation/bootstrap/) for your Git
provider. If Flux is already bootstrapped, skip this step.

For GitHub, a typical invocation is:

```bash
flux check --pre
flux bootstrap github \
  --owner=YOUR_GITHUB_USER \
  --repository=YOUR_PRIVATE_CLUSTER_CONFIG_REPO \
  --branch=main \
  --path=clusters/home \
  --personal
```

Replace those names and use `--personal` only for a personal account; use the
provider guide for organization repositories and least-privilege credentials.
Authenticate as described in the official
[GitHub bootstrap docs](https://fluxcd.io/flux/installation/bootstrap/github/);
do not paste tokens into Git, manifests, or shell history. Bootstrap installs
Flux controllers and configures reconciliation of `clusters/home` in **your
cluster-config repository**, not the Loom repository. This is a cluster-changing
command: review your current kubectl context and Git destination first.
Loom needs source-controller, helm-controller and kustomize-controller; the
standard bootstrap installs them.

### 2. Copy and customize the Loom example

In your cluster-config checkout, copy the **contents** of Loom's `deploy/flux`
directory into `clusters/home/loom/`. Keep its `kustomization.yaml`.

Edit the copied files:

1. `source.yaml`: replace `vREPLACE_WITH_STABLE_VERSION` with an existing stable
   Loom **Git tag that includes the intended chart**. Do not track `main` for a
   production install. New chart changes cannot be used from an older tag that
   predates them; wait for an appropriate release or deliberately pin a tested
   commit (`ref.commit`, replacing `ref.tag`) during evaluation.
2. `release.yaml`: replace `REPLACE_WITH_STABLE_VERSION` with the published GHCR
   **image tag without `v`**. This is separate from the Git source pin; neither
   automatically follows the other.
3. Replace `REPLACE_WITH_SQLITE_STORAGE_CLASS` with a SQLite-safe local/block
   StorageClass, or use `persistence.config.existingClaim` for an independently
   managed PVC in `media`. NFS/SMB is not appropriate for the SQLite database.
4. Before importing, set `persistence.media.enabled: true` and
   `persistence.media.existingClaim` to your prepared shared media PVC in `media`.
   Optional `persistence.downloads` and custom mounts are described in the
   Kubernetes guide. The chart's `config.filesystem.allowed_browse_roots` allows
   `/media`; add `/downloads` or replace it with `/data` if you use those mount
   paths and need to browse them in setup. Do not leave the allowlist empty.
   The example does not provision a NAS integration.

Leave ingress and Rain disabled for initial setup. The release has a **15-minute**
timeout, matching the chart's progress deadline; increase both plus startup
probe allowances if needed. All source references are in `media`, so no
cross-namespace source permissions are required.

The GitRepository URL points at public Loom source and needs no embedded
credentials. Cluster-specific secrets should be managed outside this example.
If you use Flux secret decryption, follow the official
[SOPS guide](https://fluxcd.io/flux/guides/mozilla-sops/), rather than committing
plaintext credentials or putting them in chart values.

### 3. Add it to the bootstrapped path and reconcile

In `clusters/home/kustomization.yaml` in your cluster-config repository, add
`./loom` to `resources`, preserving `./flux-system` and every existing resource:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ./flux-system
  - ./loom
```

If you use a different bootstrap path, place the directory and resource entry
there instead. This is a **Kustomize** file, not a Flux `Kustomization` custom
resource. The bootstrapped Flux `Kustomization` already reconciles this path;
you do not need an additional controller or custom resource for this small setup.
Copying files outside the reconciled path without a resource reference does not
deploy anything.

Validate locally, then commit/push **your customized cluster configuration**:

```bash
kubectl kustomize clusters/home
# Review output, commit and push in your own GitOps repository.
flux reconcile kustomization flux-system --with-source
flux get sources git -n media
flux get helmreleases -n media
kubectl -n media get pods,pvc,svc
kubectl -n media port-forward svc/loom 1925:1925
```

The example has deliberate replacement placeholders and is **not deployable
unchanged**. Rendering with Kustomize checks local composition, not remote tag
existence, image availability, storage, CRD validation or successful startup.
The source-controller packages the chart from Loom Git; helm-controller installs
it and waits for readiness. `reconcileStrategy: Revision` rebuilds the chart
when the pinned source revision changes even if `Chart.yaml`'s version did not.
There is no `HelmRepository` or fabricated OCI chart URL.

### 4. Complete setup privately

Open <http://127.0.0.1:1925> through the loopback port-forward and create the
administrator account. No default password exists; keep network access restricted
until setup is complete. Configure the external downloader and container-visible
library paths. Verify `/readyz`; after setup, add ingress/TLS through reviewed
Git changes and set `config.auth.cookie_secure: true` for HTTPS-only access.

## Updates and troubleshooting

Update the Git tag and image tag deliberately in your cluster-config repo,
review the change, then push. Flux handles Helm upgrades using the chart's
single-replica Recreate strategy, so expect downtime. Do not run a separate
`helm upgrade` against this Flux-managed release; Flux will reconcile its own
desired values over manual edits.

For backups or maintenance that require stopping Loom:

```bash
flux suspend helmrelease loom -n media
kubectl -n media scale deployment/loom --replicas=0
# Wait for termination; take a consistent backup using your storage tools.
kubectl -n media scale deployment/loom --replicas=1
flux resume helmrelease loom -n media
```

If the parent Flux Kustomization is actively restoring the HelmRelease's suspend
field, suspend that reconciliation too, or declare the maintenance suspension
in Git. Resume/revert it when finished. Keep both `loom.json` and SQLite/WAL
state together; reverting the Git image pin cannot reverse a DB migration.
Follow the Kubernetes guide's retention warnings: pruning/removing a HelmRelease
can uninstall the chart and delete chart-owned PVCs. Use externally managed
claims when you need independent retention and back up before removing resources.

Useful diagnostics:

```bash
flux get all -n media
kubectl -n media describe gitrepository loom
kubectl -n media describe helmrelease loom
kubectl -n media logs deployment/loom -c loom --tail=100
```

- Source not ready: check the actual Git tag/commit, URL and network access.
- Chart build error: confirm the pinned source contains `deploy/helm/loom`.
- Install timeout: check PVC provisioning, node placement, UID/GID permissions,
  image pull events and startup logs before simply increasing timeouts.
- Missing media: verify that the claim is mounted and paths match the downloader;
  NAS downtime must not be mistaken for an intentionally empty library.

Official references:

- [Flux getting started](https://fluxcd.io/flux/get-started/)
- [GitRepository reference](https://fluxcd.io/flux/components/source/gitrepositories/)
- [HelmRelease guide](https://fluxcd.io/flux/guides/helmreleases/)
- [HelmRelease reference](https://fluxcd.io/flux/components/helm/helmreleases/)
- [Flux Kustomization reference](https://fluxcd.io/flux/components/kustomize/kustomizations/)
