#!/usr/bin/env bash
# deploy.sh — Fast LOCAL prod deploy for Loom, fully GitOps-compatible.
#
# This is the break-glass "I need to test on prod right now" path. It does the
# exact same thing the CI workflow (.github/workflows/publish-main.yml) does —
# build the image and push it to GHCR with the tag scheme `main-<short-sha>-<ts>`
# — except the build runs on THIS machine (much faster than the self-hosted
# runners). It then nudges Flux to reconcile so the new image rolls out in
# seconds instead of waiting for the 5m + 5m image-scan / git-write intervals.
#
# Why this does NOT conflict with Flux / cause version flapping:
#   Flux image-automation in dr-homelab still fully OWNS the deployed version.
#   It selects the numerically-highest `main-<sha>-<ts>` tag from GHCR and writes
#   it into `kubernetes/apps/media/loom/release.yaml` (the {"$imagepolicy"} setter
#   marker). We push a tag with a *current* timestamp, so Flux selects it just
#   like any CI build. We never edit values, never run `helm`/`kubectl apply`,
#   never move `:latest`, never commit to the loom repo. When CI later publishes
#   a newer build, it supersedes this one automatically. No drift, no flapping.
#
#   CAVEAT: this build's tag uses the current wall-clock time, while CI uses the
#   commit's committer timestamp. If you later merge a commit whose committer
#   time predates this local deploy, CI's image won't outrank this one until a
#   newer commit lands. Harmless when you merge the same code you tested; just
#   re-run this script (or push an empty commit) if prod ever looks "stuck".
#
# Usage:
#   scripts/deploy.sh                # build, push, reconcile Flux, wait for rollout
#   scripts/deploy.sh --build-only   # build + push only; let Flux pick it up (<=10m)
#   scripts/deploy.sh --no-wait      # build, push, reconcile; don't block on rollout
#   scripts/deploy.sh -y             # skip the prod-deploy confirmation prompt
#
# Requirements: docker (buildx), git, gh (authenticated), flux, kubectl.
set -euo pipefail

# ── Config ───────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKERFILE="$REPO_ROOT/deploy/docker/Dockerfile"
BUILD_SECRETS_FILE="$REPO_ROOT/deploy/.build-secrets.env"
IMAGE="ghcr.io/ebenderooock/loom"
PLATFORM="linux/amd64"            # cluster nodes are amd64
KUBECONFIG_DEFAULT="/Users/ebenderoock/Homelab/talos/kubeconfig"

# Flux objects (all in flux-system) — see dr-homelab/kubernetes/apps/media/loom.
FLUX_NS="flux-system"
IMG_REPO="loom"                   # ImageRepository
IMG_UPDATE="loom"                 # ImageUpdateAutomation
FLUX_KS="apps"                    # Kustomization that builds ./kubernetes/apps
HELMRELEASE="loom"                # HelmRelease (targetNamespace: media)
APP_NS="media"
DEPLOY="loom"                     # Deployment name in the media namespace

# ── Flags ────────────────────────────────────────────────────────────
BUILD_ONLY=false
WAIT_ROLLOUT=true
ASSUME_YES=false
for arg in "$@"; do
  case "$arg" in
    --build-only) BUILD_ONLY=true ;;
    --no-wait)    WAIT_ROLLOUT=false ;;
    -y|--yes)     ASSUME_YES=true ;;
    -h|--help)    sed -n '2,33p' "$0"; exit 0 ;;
    *) echo "❌ Unknown argument: $arg (try --help)" >&2; exit 2 ;;
  esac
done

# ── Helpers ──────────────────────────────────────────────────────────
die()  { echo "❌ $*" >&2; exit 1; }
info() { echo "▸ $*"; }
ok()   { echo "✅ $*"; }

for bin in docker git gh flux kubectl; do
  command -v "$bin" >/dev/null 2>&1 || die "Required tool not found on PATH: $bin"
done

# ── 1. Build metadata + tag ──────────────────────────────────────────
cd "$REPO_ROOT"
COMMIT_FULL="$(git rev-parse HEAD)"
COMMIT="$(git rev-parse --short HEAD)"
VERSION="$(git describe --tags --always 2>/dev/null || echo "$COMMIT")"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
# Current wall-clock unix time → guarantees this build outranks the last CI build
# in Flux's numerical ImagePolicy (matches the `main-<sha>-<ts>` tag pattern).
TS="$(date +%s)"
TAG="main-${COMMIT}-${TS}"
FULL_IMAGE="${IMAGE}:${TAG}"

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "⚠️  Working tree has uncommitted changes — the image is built from your"
  echo "    LOCAL tree but tagged with commit ${COMMIT}. Fine for quick testing;"
  echo "    the tag won't exactly correspond to that commit's source."
fi

echo
echo "  Image:   ${FULL_IMAGE}"
echo "  Version: ${VERSION}"
echo "  Commit:  ${COMMIT_FULL}"
echo "  Target:  PRODUCTION (media namespace via Flux)"
echo
if ! $ASSUME_YES && [ -t 0 ]; then
  read -r -p "Deploy this to PROD? [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || die "Aborted."
fi

# ── 2. Load build secrets (TVDB_APIKEY baked into the image) ─────────
if [[ -f "$BUILD_SECRETS_FILE" ]]; then
  info "Loading build secrets from $(basename "$BUILD_SECRETS_FILE")"
  # shellcheck disable=SC1090
  set -a; source "$BUILD_SECRETS_FILE"; set +a
fi
: "${TVDB_APIKEY:=}"

# ── 3. Log in to GHCR (best-effort, via gh token) ────────────────────
GH_USER="$(gh api user -q .login 2>/dev/null || echo Ebenderooock)"
if gh auth token >/dev/null 2>&1; then
  info "Logging in to ghcr.io as ${GH_USER}"
  gh auth token | docker login ghcr.io -u "$GH_USER" --password-stdin >/dev/null 2>&1 \
    || echo "⚠️  docker login via gh token failed; relying on existing credentials."
  echo "   (If the push below fails with 'denied', your gh token lacks the"
  echo "    write:packages scope — run: gh auth refresh -s write:packages)"
fi

# ── 4. Build + push (same Dockerfile/args as publish-main.yml) ───────
info "Building and pushing ${FULL_IMAGE} (${PLATFORM}) ..."
docker buildx build \
  --platform "$PLATFORM" \
  --file "$DOCKERFILE" \
  --build-arg "VERSION=${VERSION}" \
  --build-arg "COMMIT=${COMMIT_FULL}" \
  --build-arg "DATE=${DATE}" \
  --build-arg "TVDB_APIKEY=${TVDB_APIKEY}" \
  --tag "$FULL_IMAGE" \
  --push \
  "$REPO_ROOT"
ok "Pushed ${FULL_IMAGE}"

if $BUILD_ONLY; then
  echo
  ok "Build-only mode: Flux will scan GHCR and roll this out within ~10m."
  echo "   (Run without --build-only to reconcile immediately.)"
  exit 0
fi

# ── 5. Reconcile Flux so the new image deploys now ───────────────────
export KUBECONFIG="${KUBECONFIG:-$KUBECONFIG_DEFAULT}"
[ -f "$KUBECONFIG" ] || die "KUBECONFIG not found: $KUBECONFIG (is the VPN up?)"

info "Reconciling Flux image-automation ..."
# Scan GHCR, then force the ImagePolicy to re-select against the fresh scan
# BEFORE image-update reads .status.latestImage (otherwise it can act on a stale
# selection and commit the previous tag).
flux reconcile image repository "$IMG_REPO" -n "$FLUX_NS" --timeout 2m \
  || die "image repository reconcile failed (VPN / cluster reachable?)"
flux reconcile image policy "$IMG_REPO" -n "$FLUX_NS" --timeout 2m \
  || die "image policy reconcile failed"
flux reconcile image update "$IMG_UPDATE" -n "$FLUX_NS" --timeout 2m \
  || die "image update reconcile failed"

info "Reconciling Kustomization + HelmRelease ..."
# Pull the auto-commit and apply the HelmRelease change, then force the upgrade.
flux reconcile kustomization "$FLUX_KS" -n "$FLUX_NS" --with-source --timeout 3m \
  || die "kustomization reconcile failed"
flux reconcile helmrelease "$HELMRELEASE" -n "$FLUX_NS" --timeout 5m \
  || die "helmrelease reconcile failed"

# ── 6. Wait for the new image to converge + verify ──────────────────
# `kubectl rollout status` alone is unsafe here: right after the reconcile the
# Deployment may still carry the OLD image, so rollout status would return
# success for the previous revision. Poll until Flux has actually written THIS
# image into the Deployment spec, THEN wait for the rollout, THEN hard-verify.
if $WAIT_ROLLOUT; then
  info "Waiting for Flux to roll out ${TAG} ..."
  img_path='{.spec.template.spec.containers[?(@.name=="loom")].image}'
  deadline=$((SECONDS + 240))
  while true; do
    running_img="$(kubectl -n "$APP_NS" get "deployment/${DEPLOY}" \
      -o jsonpath="$img_path" 2>/dev/null || true)"
    if [[ "$running_img" == "$FULL_IMAGE" ]]; then
      break
    fi
    if (( SECONDS >= deadline )); then
      die "timed out waiting for Deployment image to become ${FULL_IMAGE} (still '${running_img:-none}'). Check: flux get all -A | grep loom"
    fi
    sleep 5
  done
  kubectl -n "$APP_NS" rollout status "deployment/${DEPLOY}" --timeout=180s \
    || die "rollout did not complete"
  ok "Loom ${TAG} is live in the ${APP_NS} namespace (image: ${FULL_IMAGE})."
else
  ok "Reconcile triggered. Skipping rollout wait (--no-wait)."
  echo "   Verify with: kubectl -n ${APP_NS} get deploy/${DEPLOY} -o jsonpath='{.spec.template.spec.containers[?(@.name==\"loom\")].image}'"
fi
