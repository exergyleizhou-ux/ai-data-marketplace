#!/usr/bin/env bash
# Idempotently make the `kmeans` C2D algorithm runnable on the demo marketplace,
# durably: a PERSISTENT local registry (named volume + auto-restart) holds the
# digest-pinned image, the algorithm is registered+trusted, and the demo dataset's
# offer allows it. Re-run after any restart / DB reset to restore the full state;
# re-running is a no-op once everything is in place (exactly one kmeans survives).
#
#   scripts/demo-kmeans-up.sh
#
# Env (all have demo defaults):
#   API=http://localhost:8080/api/v1   PASSWORD=Oasis1234!
#   OPS_ACCT=demo-ops@oasis.test   SELLER_ACCT=demo-seller@oasis.test
#   DATASET_ID=81775902-feb8-4dc2-9586-202c9ce0f75b   ALGO=kmeans   NAME="K-Means 聚类"
#   REBUILD=1  # force a rebuild+push even if the registry already has the image
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
PW="${PASSWORD:-Oasis1234!}"
OPS_ACCT="${OPS_ACCT:-demo-ops@oasis.test}"
SELLER_ACCT="${SELLER_ACCT:-demo-seller@oasis.test}"
DATASET_ID="${DATASET_ID:-81775902-feb8-4dc2-9586-202c9ce0f75b}"
ALGO="${ALGO:-kmeans}"; NAME="${NAME:-K-Means 聚类}"
REG="127.0.0.1:5000"; REG_NAME="oasis-registry"; VOL="oasis-registry-data"
HERE="$(cd "$(dirname "$0")/.." && pwd)"; IMG="$REG/vo-$ALGO"
REPO="https://github.com/exergyleizhou-ux/ai-data-marketplace/tree/main/algorithms/$ALGO"
ACCEPT='Accept: application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json,application/vnd.oci.image.index.v1+json'

login() { curl -fsS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"account\":\"$1\",\"password\":\"$PW\"}" \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).data.tokens.access_token))'; }
served_digest() { curl -fsSI -H "$ACCEPT" "http://$REG/v2/vo-$ALGO/manifests/1" 2>/dev/null \
  | grep -i docker-content-digest | awk '{print $2}' | tr -d '\r'; }

echo "▸ persistent registry ($REG, volume=$VOL, restart=unless-stopped)…"
if ! docker inspect "$REG_NAME" >/dev/null 2>&1; then
  docker run -d -p "$REG:5000" --restart unless-stopped -v "$VOL:/var/lib/registry" --name "$REG_NAME" registry:2 >/dev/null
else
  docker update --restart unless-stopped "$REG_NAME" >/dev/null 2>&1 || true
fi
for _ in $(seq 1 15); do curl -fsS "http://$REG/v2/" >/dev/null 2>&1 && break; sleep 1; done

# Stable digest: only build+push when the registry doesn't already serve the image
# (or REBUILD=1). Avoids churn from non-reproducible Docker builds → idempotent.
DIGEST="$(served_digest || true)"
if [ -z "$DIGEST" ] || [ "${REBUILD:-0}" = "1" ]; then
  echo "▸ build + push $IMG:1…"
  docker build -q -t "$IMG:1" "$HERE/algorithms/$ALGO" >/dev/null
  docker push -q "$IMG:1" >/dev/null
  DIGEST="$(served_digest)"
else
  echo "▸ image already in registry, reusing it"
fi
[ -n "$DIGEST" ] || { echo "no manifest digest" >&2; exit 1; }
echo "  digest: $DIGEST"

OPS="$(login "$OPS_ACCT")"
curl -fsS -H "Authorization: Bearer $OPS" "$API/admin/compute/algorithms?status=approved" > /tmp/.algos.json
# Reuse an already-trusted algo with this exact digest; else register + approve.
AID="$(NAME="$NAME" DIGEST="$DIGEST" node -e '
const j=require("/tmp/.algos.json");const a=(j.data.items||[]).find(x=>x.name===process.env.NAME&&x.image_digest===process.env.DIGEST&&x.trusted);process.stdout.write(a?a.id:"")')"
if [ -z "$AID" ]; then
  echo "▸ register + approve…"
  AID="$(curl -fsS -X POST "$API/admin/compute/algorithms" -H "Authorization: Bearer $OPS" -H 'Content-Type: application/json' \
    -d "{\"name\":\"$NAME\",\"runtime\":\"python-sklearn\",\"image\":\"$IMG\",\"image_digest\":\"$DIGEST\",\"source_ref\":\"$REPO\",\"output_kind\":\"model\",\"version\":1}" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).data.id))')"
  curl -fsS -X POST "$API/admin/compute/algorithms/$AID/review" -H "Authorization: Bearer $OPS" -H 'Content-Type: application/json' \
    -d '{"status":"approved","trusted":true}' >/dev/null
fi
echo "  algorithm id: $AID"

echo "▸ retire stale same-name algorithms (keep exactly one)…"
for SID in $(NAME="$NAME" AID="$AID" node -e '
const j=require("/tmp/.algos.json");process.stdout.write((j.data.items||[]).filter(x=>x.name===process.env.NAME&&x.id!==process.env.AID).map(x=>x.id).join("\n"))'); do
  curl -fsS -X POST "$API/admin/compute/algorithms/$SID/review" -H "Authorization: Bearer $OPS" -H 'Content-Type: application/json' \
    -d '{"status":"rejected","trusted":false}' >/dev/null && echo "  retired $SID"
done

echo "▸ add to dataset offer allowlist (as seller, idempotent)…"
SELLER="$(login "$SELLER_ACCT")"
curl -fsS -H "Authorization: Bearer $SELLER" "$API/datasets/$DATASET_ID/compute-offer" > /tmp/.offer.json
AID="$AID" node -e '
const o=require("/tmp/.offer.json").data, aid=process.env.AID;
const ids=new Set(o.allowed_algorithm_ids||[]); ids.add(aid);
require("fs").writeFileSync("/tmp/.offer.put.json",JSON.stringify({
  enabled:o.enabled,allow_custom:o.allow_custom,allowed_algorithm_ids:[...ids],
  price_cents:o.price_cents,max_runtime_secs:o.max_runtime_secs,max_output_bytes:o.max_output_bytes,
  max_output_files:o.max_output_files,return_logs:o.return_logs,review_output:o.review_output,
  trust_level:o.trust_level,allow_federated:o.allow_federated,allow_psi:o.allow_psi}));'
curl -fsS -X PUT "$API/datasets/$DATASET_ID/compute-offer" -H "Authorization: Bearer $SELLER" -H 'Content-Type: application/json' \
  -d @/tmp/.offer.put.json >/dev/null
rm -f /tmp/.algos.json /tmp/.offer.json /tmp/.offer.put.json
echo "✓ kmeans runnable on dataset $DATASET_ID (algo $AID, digest $DIGEST)"
