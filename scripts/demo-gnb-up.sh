#!/usr/bin/env bash
# Make the `gnb` (Gaussian Naive Bayes) C2D algorithm runnable on the demo
# marketplace, durably — same flow as demo-kmeans-up.sh (persistent registry,
# digest-pinned trusted algo, dataset offer allowlist). A thin wrapper that sets
# the gnb identity and delegates to the shared script.
#
#   scripts/demo-gnb-up.sh
#
# Env (demo defaults): NAME, DATASET_ID — see demo-kmeans-up.sh for the rest.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ALGO=gnb \
  NAME="${NAME:-高斯朴素贝叶斯 (GNB)}" \
  DATASET_ID="${DATASET_ID:-2e3896e2-bac3-4e34-b9c1-4290b954b981}" \
  exec "$HERE/demo-kmeans-up.sh"
