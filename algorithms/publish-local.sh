#!/usr/bin/env bash
# Publish a C2D algorithm to a LOCAL registry and print everything needed to
# register it on a running Oasis instance — the reproducible version of the
# manual steps used to add `kmeans` live.
#
# Why a local registry: an L1 model-output algorithm must be TRUSTED, and trust
# requires a pinned image_digest (design §4/§7.3). A locally-built image has no
# registry manifest digest, so we push to a local registry to get a real digest
# WITHOUT needing Docker Hub / cloud creds. For production, push to your real
# registry instead and use that digest (see docs/部署-C2D算法镜像与生产Runner.md).
#
# Usage:
#   algorithms/publish-local.sh <algo-dir> [name]
#   # e.g. algorithms/publish-local.sh kmeans "K-Means 聚类"
#
# Optional auto-register (else it just prints the curl commands):
#   OPS_TOKEN=<jwt> API=http://localhost:8080/api/v1 algorithms/publish-local.sh kmeans
set -euo pipefail

ALGO_DIR="${1:?usage: publish-local.sh <algo-dir> [name]}"
NAME="${2:-$ALGO_DIR}"
REG="${REGISTRY:-127.0.0.1:5000}"      # IPv4 — 'localhost' can resolve to ::1 and time out
REG_NAME="${REG_CONTAINER:-oasis-registry}"
HERE="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$HERE/$ALGO_DIR"
REPO="github.com/exergyleizhou-ux/ai-data-marketplace/tree/main/algorithms/$ALGO_DIR"
IMG="$REG/vo-$ALGO_DIR"

[ -f "$SRC_DIR/Dockerfile" ] || { echo "no Dockerfile in $SRC_DIR" >&2; exit 1; }

echo "▸ ensure local registry ($REG)…"
if ! curl -fsS "http://$REG/v2/" >/dev/null 2>&1; then
  docker rm -f "$REG_NAME" >/dev/null 2>&1 || true
  docker run -d -p "${REG%:*}:${REG#*:}:5000" --name "$REG_NAME" registry:2 >/dev/null
  for _ in $(seq 1 15); do curl -fsS "http://$REG/v2/" >/dev/null 2>&1 && break; sleep 1; done
fi

echo "▸ build + push $IMG:1…"
docker build -q -t "$IMG:1" "$SRC_DIR" >/dev/null
docker push -q "$IMG:1" >/dev/null
DIGEST="$(docker inspect --format='{{range .RepoDigests}}{{println .}}{{end}}' "$IMG:1" | grep "$REG" | sed 's/.*@//' | head -1)"
[ -n "$DIGEST" ] || { echo "could not resolve manifest digest" >&2; exit 1; }
echo "  image:  $IMG"
echo "  digest: $DIGEST"

PAYLOAD=$(printf '{"name":"%s","runtime":"python-sklearn","image":"%s","image_digest":"%s","source_ref":"https://%s","output_kind":"model","version":1}' \
  "$NAME" "$IMG" "$DIGEST" "$REPO")

if [ -n "${OPS_TOKEN:-}" ] && [ -n "${API:-}" ]; then
  echo "▸ registering + approving via $API…"
  ID=$(curl -fsS -X POST "$API/admin/compute/algorithms" -H "Authorization: Bearer $OPS_TOKEN" -H 'Content-Type: application/json' -d "$PAYLOAD" \
        | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
  echo "  algorithm id: $ID"
  curl -fsS -X POST "$API/admin/compute/algorithms/$ID/review" -H "Authorization: Bearer $OPS_TOKEN" -H 'Content-Type: application/json' \
    -d '{"status":"approved","trusted":true}' >/dev/null && echo "  approved + trusted ✓"
  echo "  → now add $ID to the dataset's offer allowed_algorithm_ids (as the seller)."
else
  echo
  echo "Register it (as ops):"
  echo "  curl -X POST \$API/admin/compute/algorithms -H \"Authorization: Bearer \$OPS_TOKEN\" \\"
  echo "    -H 'Content-Type: application/json' -d '$PAYLOAD'"
  echo "Then POST /admin/compute/algorithms/<id>/review {\"status\":\"approved\",\"trusted\":true},"
  echo "and add <id> to the dataset offer's allowed_algorithm_ids as the seller."
fi
