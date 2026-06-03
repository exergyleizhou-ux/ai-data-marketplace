#!/usr/bin/env bash
# Build + push the Compute-to-Data sandbox algorithm images to the PRODUCTION
# registry and print the sha256 digest to pin when registering each algorithm.
#
# Trusted algorithms MUST be pinned by DIGEST, never a mutable tag (design §4 /
# §7.3) — a `:latest` that moves would silently void the platform's audit.
#
# Usage (after `docker login` to the registry account):
#   REGISTRY=docker.io/yes0505/c2d-algorithm ./algorithms/publish.sh
#
# NOTE: always brace ${REGISTRY}:${tag} — a bare $REGISTRY:tag hits zsh's `:l`
# history modifier and mangles the tag.
set -euo pipefail

REGISTRY="${REGISTRY:-docker.io/yes0505/c2d-algorithm}"
HERE="$(cd "$(dirname "$0")" && pwd)"

publish() { # name  dir  tag
  local name="$1" dir="$2" tag="$3"
  echo ">> building ${name}  →  ${REGISTRY}:${tag}"
  docker build -q -t "${REGISTRY}:${tag}" "${HERE}/${dir}" >/dev/null
  docker push "${REGISTRY}:${tag}" >/dev/null
  local repodigest
  repodigest="$(docker inspect --format='{{index .RepoDigests 0}}' "${REGISTRY}:${tag}")"
  printf '   %-9s image=%s  digest=%s\n' "${name}" "${REGISTRY}" "${repodigest#*@}"
}

publish logreg     logreg     "logreg-1.0.0"
publish dp_stats   dp_stats   "dp-stats-1.0.0"
publish fed-logreg fed-logreg "fed-logreg-1.0.0"

echo
echo "Register each via the ops API with image=${REGISTRY}, image_digest=<digest above>,"
echo "output_kind (logreg=model, dp_stats=aggregate, fed-logreg=model), then approve with"
echo "trusted=true. fed-logreg uses runtime=fed-logreg and is driven by"
echo "POST /compute/federated-jobs (one sub-job per dataset → FedAvg)."
echo "Run the platform with COMPUTE_RUNNER=docker (+ optional COMPUTE_DOCKER_RUNTIME=runsc)."
