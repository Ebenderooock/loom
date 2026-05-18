#!/usr/bin/env bash
#
# Setup GitHub Actions Runner Controller (ARC) on Kubernetes
# Repo: Ebenderooock/loom
#
# Prerequisites:
#   - kubectl configured and pointing at your cluster
#   - Helm 3 installed
#   - A GitHub PAT with repo scope (or a GitHub App)
#
set -euo pipefail

# ─── Configuration ──────────────────────────────────────────────
NAMESPACE="actions-runner-system"
GITHUB_ORG="Ebenderooock"  # org/user-level — available to all repos
MIN_RUNNERS=1
MAX_RUNNERS=5
RUNNER_IMAGE="ghcr.io/actions/actions-runner:latest"

# You MUST set this before running (or export it in your shell):
#   export GITHUB_PAT="ghp_xxxxxxxxxxxx"
if [[ -z "${GITHUB_PAT:-}" ]]; then
  echo "ERROR: Set GITHUB_PAT environment variable first."
  echo "  export GITHUB_PAT='ghp_your_token_here'"
  echo ""
  echo "Required scopes: admin:org (org runners) or manage_runners:org"
  exit 1
fi

echo "==> Creating namespace ${NAMESPACE}"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# ─── Step 1: Install ARC controller ────────────────────────────
echo "==> Adding GitHub Actions Helm chart repo"
helm repo add actions-runner-controller \
  https://actions-runner-controller.github.io/actions-runner-controller 2>/dev/null || true
helm repo update

echo "==> Installing ARC controller"
helm upgrade --install arc \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller \
  --namespace "${NAMESPACE}" \
  --wait

# ─── Step 2: Create the runner scale set ────────────────────────
echo "==> Installing runner scale set for ${GITHUB_ORG} (all repos)"
helm upgrade --install arc-runners \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set \
  --namespace "${NAMESPACE}" \
  --set githubConfigUrl="https://github.com/${GITHUB_ORG}" \
  --set githubConfigSecret.github_token="${GITHUB_PAT}" \
  --set minRunners="${MIN_RUNNERS}" \
  --set maxRunners="${MAX_RUNNERS}" \
  --set containerMode.type="dind" \
  --wait

# ─── Step 3: Verify ────────────────────────────────────────────
echo ""
echo "==> Verifying deployment"
kubectl -n "${NAMESPACE}" get pods
echo ""
echo "==> Runner scale set:"
kubectl -n "${NAMESPACE}" get autoscalingrunnersets

cat <<EOF

✅ ARC runners deployed successfully!

Usage in any repo workflow:
  jobs:
    build:
      runs-on: arc-runners   # matches the scale set name

Useful commands:
  kubectl -n ${NAMESPACE} get pods               # check runner pods
  kubectl -n ${NAMESPACE} logs -l app=arc-runners   # view logs
  kubectl -n ${NAMESPACE} get autoscalingrunnersets  # check scaling

To uninstall:
  helm uninstall arc-runners -n ${NAMESPACE}
  helm uninstall arc -n ${NAMESPACE}

EOF
