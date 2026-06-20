#!/usr/bin/env bash
# Register the PaperGuard integrity-screen algorithm as a TRUSTED compute
# algorithm — the one prerequisite the self-serve Verify API (POST /api/v1/screen)
# needs on a fresh deploy. `ScreenAdhoc`'s `trustedScreener` looks up a trusted
# algorithm whose name/image contains "paperguard"; without this, /screen returns
# "no trusted integrity-screen algorithm registered".
#
# Idempotent: re-run after any restart / DB reset. No marketplace dataset/offer is
# touched (the Verify path is self-serve, not marketplace-bound).
#
#   scripts/register-verify-screener.sh
#
# Env (all have sensible defaults):
#   API=http://localhost:8080/api/v1   PASSWORD=Oasis1234!   OPS_ACCT=demo-ops@oasis.test
#   REBUILD=1  # force rebuild+push even if the registry already serves the image
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
PW="${PASSWORD:-Oasis1234!}"
OPS_ACCT="${OPS_ACCT:-demo-ops@oasis.test}"
ALGO="paperguard"; NAME="${NAME:-PaperGuard data-integrity screen}"
REG="${REG:-127.0.0.1:5000}"; REG_NAME="oasis-registry"; VOL="oasis-registry-data"
HERE="$(cd "$(dirname "$0")/.." && pwd)"; IMG="$REG/vo-$ALGO"
REPO="https://github.com/exergyleizhou-ux/ai-data-marketplace/tree/main/algorithms/$ALGO"
ACCEPT='Accept: application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json,application/vnd.oci.image.index.v1+json'

login() { curl -fsS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"account\":\"$1\",\"password\":\"$PW\"}" \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).data.tokens.access_token))'; }
served_digest() { curl -fsSI -H "$ACCEPT" "http://$REG/v2/vo-$ALGO/manifests/1" 2>/dev/null \
  | grep -i docker-content-digest | awk '{print $2}' | tr -d '\r'; }

echo "▸ persistent registry ($REG, volume=$VOL)…"
if ! docker inspect "$REG_NAME" >/dev/null 2>&1; then
  docker run -d -p "$REG:5000" --restart unless-stopped -v "$VOL:/var/lib/registry" --name "$REG_NAME" registry:2 >/dev/null
else
  docker update --restart unless-stopped "$REG_NAME" >/dev/null 2>&1 || true
fi
for _ in $(seq 1 15); do curl -fsS "http://$REG/v2/" >/dev/null 2>&1 && break; sleep 1; done

DIGEST="$(served_digest || true)"
if [ -z "$DIGEST" ] || [ "${REBUILD:-0}" = "1" ]; then
  echo "▸ build + push $IMG:1 (tuna PyPI mirror to avoid proxy wheel corruption)…"
  docker build -q --build-arg PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple \
    -t "$IMG:1" "$HERE/algorithms/$ALGO" >/dev/null
  docker push -q "$IMG:1" >/dev/null
  DIGEST="$(served_digest)"
else
  echo "▸ image already in registry, reusing it"
fi
[ -n "$DIGEST" ] || { echo "no manifest digest" >&2; exit 1; }
echo "  digest: $DIGEST"

OPS="$(login "$OPS_ACCT")"
curl -fsS -H "Authorization: Bearer $OPS" "$API/admin/compute/algorithms?status=approved" > /tmp/.vscreen.json
AID="$(NAME="$NAME" DIGEST="$DIGEST" node -e '
const j=require("/tmp/.vscreen.json");const a=(j.data.items||[]).find(x=>x.name===process.env.NAME&&x.image_digest===process.env.DIGEST&&x.trusted);process.stdout.write(a?a.id:"")')"
if [ -z "$AID" ]; then
  echo "▸ register + approve + trust…"
  AID="$(curl -fsS -X POST "$API/admin/compute/algorithms" -H "Authorization: Bearer $OPS" -H 'Content-Type: application/json' \
    -d "{\"name\":\"$NAME\",\"runtime\":\"python-sklearn\",\"image\":\"$IMG\",\"image_digest\":\"$DIGEST\",\"source_ref\":\"$REPO\",\"output_kind\":\"model\",\"version\":1}" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).data.id))')"
  curl -fsS -X POST "$API/admin/compute/algorithms/$AID/review" -H "Authorization: Bearer $OPS" -H 'Content-Type: application/json' \
    -d '{"status":"approved","trusted":true}' >/dev/null
fi
rm -f /tmp/.vscreen.json
echo "✓ Verify screener ready (algo $AID, digest $DIGEST). POST /api/v1/screen now works."
