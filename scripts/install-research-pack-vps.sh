#!/usr/bin/env bash
# Install/update Claude Science research pack on the demo VPS.
# Source: local ~/.claude-science (macOS arm64). Rebuilds Linux operon-mcp venv
# because Mac conda binaries cannot run on linux/amd64.
#
#   bash scripts/install-research-pack-vps.sh
set -euo pipefail

VPS_HOST="${VPS_HOST:-118.31.47.129}"
VPS_USER="${VPS_USER:-root}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/oasis_deploy}"
CS="${CS_SRC:-$HOME/.claude-science}"
TARGET=/root/.lumen/science/sandbox/home/.claude-science
RSH="ssh -i $SSH_KEY -o StrictHostKeyChecking=no"

if [[ ! -d "$CS/runtime" ]]; then
  echo "missing $CS/runtime — install Claude Science locally first" >&2
  exit 1
fi

ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${VPS_USER}@${VPS_HOST}" "mkdir -p $TARGET"

for part in runtime bin seed-assets; do
  echo "▸ rsync $part"
  rsync -az -e "$RSH" "$CS/$part/" "${VPS_USER}@${VPS_HOST}:$TARGET/$part/"
done
# Do NOT rsync Mac conda env — rebuild on Linux below.
rsync -az -e "$RSH" "$CS/active-org.json" "$CS/install-id" \
  "${VPS_USER}@${VPS_HOST}:$TARGET/" 2>/dev/null || true

echo "▸ rebuild Linux operon-mcp venv + restart lab"
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${VPS_USER}@${VPS_HOST}" bash -s <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq python3-venv python3-pip >/dev/null
DATA=/root/.lumen/science/sandbox/home/.claude-science
ENVDIR=$DATA/conda/envs/operon-mcp
rm -rf "$ENVDIR"
mkdir -p "$DATA/conda/envs"
python3 -m venv "$ENVDIR"
# shellcheck disable=SC1091
source "$ENVDIR/bin/activate"
pip install -q -U pip wheel
pip install -q "mcp>=1.27" anyio httpx httpx-sse pydantic pydantic-settings \
  python-dotenv python-multipart requests urllib3 jsonschema \
  sse-starlette starlette uvicorn numpy pandas PyJWT cryptography
ln -sfn python3 "$ENVDIR/bin/python3.13"
BT=$(find "$DATA/runtime" -type d -name bio-tools | head -1)
cd "$BT"
python -c "import sys; sys.path.insert(0,'lib'); import mcp_pubmed.server; print('IMPORT_OK')"
systemctl restart lumen-lab
sleep 8
curl -fsS http://127.0.0.1:18992/api/lab/health | python3 -c "
import sys,json
d=json.load(sys.stdin)
f=d['fleet']; r=d['research_pack']
print('research_healthy', r.get('healthy'), 'domains', r.get('domains'), 'tools', r.get('domain_tools'))
print('cs_connected', f.get('cs_connected'), '/', f.get('cs_domains'), 'native', f.get('lumen_native'))
print('errors', len(f.get('errors') or {}))
"
REMOTE

echo "✓ research pack installed on ${VPS_HOST}"
