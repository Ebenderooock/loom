#!/usr/bin/env bash
set -euo pipefail

# Loom — build and push container image to GHCR
# Run from the repo root on your Mac.
#
# Prerequisites:
#   1. Docker Desktop running
#   2. Authenticated to GHCR:
#      echo YOUR_GITHUB_PAT | docker login ghcr.io -u ebenderooock --password-stdin
#      (PAT needs write:packages scope)

REPO="ghcr.io/ebenderooock/loom"
VERSION="${1:-0.1.0}"
COMMIT="$(git rev-parse --short HEAD)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PLATFORM="${2:-linux/amd64}"  # use "linux/amd64,linux/arm64" for multi-arch

echo "==> Building Loom ${VERSION} (commit: ${COMMIT}, platform: ${PLATFORM})"

docker build \
  -f deploy/docker/Dockerfile \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  --build-arg DATE="${DATE}" \
  -t "${REPO}:${VERSION}" \
  -t "${REPO}:latest" \
  .

echo "==> Pushing ${REPO}:${VERSION}"
docker push "${REPO}:${VERSION}"
docker push "${REPO}:latest"

echo "==> Pushed ${REPO}:${VERSION} and ${REPO}:latest"
echo ""
echo "Deploy with:"
echo "  helm install loom ./deploy/helm/loom -f deploy/helm/values-homelab.yaml -n media --create-namespace"
