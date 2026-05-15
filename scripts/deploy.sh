#!/usr/bin/env bash
# deploy.sh — Build, push, and deploy Loom to the homelab Kubernetes cluster.
#
# Usage:
#   ./scripts/deploy.sh            # auto-increment patch version
#   ./scripts/deploy.sh 0.2.0      # explicit version override
#
# Requirements:
#   - docker (with buildx)
#   - helm
#   - kubectl (configured for the target cluster)
#   - gh / git (authenticated to push to ghcr.io and the repo)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VALUES_FILE="$REPO_ROOT/deploy/helm/values-homelab.yaml"
CHART_DIR="$REPO_ROOT/deploy/helm/loom"
DOCKERFILE="$REPO_ROOT/deploy/docker/Dockerfile"
IMAGE="ghcr.io/ebenderooock/loom"
NAMESPACE="media"
RELEASE="loom"
BRANCH="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"

# ── Helpers ──────────────────────────────────────────────────────────

die()  { echo "❌ $*" >&2; exit 1; }
info() { echo "▸ $*"; }

# ── 1. Resolve version ──────────────────────────────────────────────

current_tag=$(grep -E '^[[:space:]]*tag:' "$VALUES_FILE" | head -1 | sed 's/.*tag:[[:space:]]*"\{0,1\}\([0-9][0-9.]*\)"\{0,1\}.*/\1/')
[ -n "$current_tag" ] || die "Could not read current tag from $VALUES_FILE"

if [ $# -ge 1 ]; then
  NEW_TAG="$1"
  info "Using explicit version: $NEW_TAG"
else
  # Auto-increment patch: 0.1.37 → 0.1.38
  IFS='.' read -r major minor patch <<< "$current_tag"
  patch=$((patch + 1))
  NEW_TAG="${major}.${minor}.${patch}"
  info "Auto-incremented version: $current_tag → $NEW_TAG"
fi

COMMIT=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ── 2. Build Docker image ───────────────────────────────────────────

info "Building Docker image ${IMAGE}:${NEW_TAG} ..."
docker build \
  --platform linux/amd64 \
  --build-arg VERSION="$NEW_TAG" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg DATE="$DATE" \
  -t "${IMAGE}:${NEW_TAG}" \
  -t "${IMAGE}:latest" \
  -f "$DOCKERFILE" \
  "$REPO_ROOT"

# ── 3. Push to GHCR ─────────────────────────────────────────────────

info "Pushing ${IMAGE}:${NEW_TAG} ..."
docker push "${IMAGE}:${NEW_TAG}"
docker push "${IMAGE}:latest"

# ── 4. Update values-homelab.yaml ────────────────────────────────────

info "Updating $VALUES_FILE → tag: \"${NEW_TAG}\""
if [[ "$OSTYPE" == darwin* ]]; then
  sed -i '' "s/tag: \"${current_tag}\"/tag: \"${NEW_TAG}\"/" "$VALUES_FILE"
else
  sed -i "s/tag: \"${current_tag}\"/tag: \"${NEW_TAG}\"/" "$VALUES_FILE"
fi

# ── 5. Commit and push version bump ─────────────────────────────────

info "Committing version bump ..."
git -C "$REPO_ROOT" add "$VALUES_FILE"
git -C "$REPO_ROOT" commit -m "chore: bump image to ${NEW_TAG} [skip ci]"
git -C "$REPO_ROOT" push origin "$BRANCH"

# ── 6. Deploy to Kubernetes ──────────────────────────────────────────

info "Deploying ${RELEASE} to namespace ${NAMESPACE} ..."
helm upgrade --install "$RELEASE" "$CHART_DIR" \
  -f "$VALUES_FILE" \
  -n "$NAMESPACE" \
  --create-namespace \
  --wait \
  --timeout 120s

# ── 7. Verify ────────────────────────────────────────────────────────

info "Waiting for rollout ..."
kubectl rollout status deployment/"$RELEASE" -n "$NAMESPACE" --timeout=90s

POD=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=loom -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [ -n "$POD" ]; then
  info "Pod: $POD"
  kubectl get pod "$POD" -n "$NAMESPACE" -o wide
fi

echo ""
echo "✅ Loom ${NEW_TAG} deployed to ${NAMESPACE} namespace."
