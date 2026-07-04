#!/usr/bin/env bash
# Native (no-Docker) one-command deploy of the Oasis verified-data-marketplace
# demo to a fresh Ubuntu VPS. Cross-compiles the Go backend locally, ships the
# binary + frontend source, then installs Postgres/Node, builds the frontend,
# wires systemd services, and seeds the 8 public research datasets.
#
# Designed for a small box (2 vCPU / 2-4 GiB): adds swap so the Next build never
# OOMs, and uses apt + the seeder's own download path (both inbound = uncapped on
# most clouds). Use an overseas/HK region to avoid mainland-China ICP 备案, which
# resets inbound HTTP to un-filed instances.
#
# Usage:
#   scripts/deploy-native.sh <public_ip> [ssh_user] [ssh_key]
# Example:
#   scripts/deploy-native.sh 47.x.x.x root ~/.ssh/oasis_deploy
set -euo pipefail

IP="${1:?usage: deploy-native.sh <public_ip> [ssh_user] [ssh_key]}"
USER_="${2:-root}"
KEY="${3:-$HOME/.ssh/oasis_deploy}"
HOST="$USER_@$IP"
SSH="ssh -i $KEY -o StrictHostKeyChecking=accept-new -o BatchMode=yes"
SCP="scp -i $KEY -o StrictHostKeyChecking=accept-new -o BatchMode=yes"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> [1/6] cross-compile backend (linux/amd64)"
( cd "$REPO_ROOT/backend"
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$WORK/api" ./cmd/api
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$WORK/seedpublic" ./cmd/seedpublic )
gzip -1 "$WORK/api" "$WORK/seedpublic"

echo "==> [2/6] package frontend source"
tar --exclude=node_modules --exclude=.next --exclude=.git --exclude='.env*' \
    -czf "$WORK/frontend-src.tgz" -C "$REPO_ROOT" frontend

echo "==> [3/6] upload to $HOST"
$SSH "$HOST" 'mkdir -p /opt/oasis/storage'
$SCP "$WORK/api.gz" "$WORK/seedpublic.gz" "$WORK/frontend-src.tgz" "$HOST:/opt/oasis/"

echo "==> [4/6] install + configure on server (swap, postgres, node)"
$SSH "$HOST" "PUBLIC_IP=$IP bash -s" <<'REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
# swap (idempotent)
if ! swapon --show | grep -q .; then
  fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
apt-get update -qq
apt-get install -y -qq postgresql nodejs npm >/dev/null
cd /opt/oasis
gunzip -f api.gz seedpublic.gz && chmod +x api seedpublic
rm -rf frontend && tar xzf frontend-src.tgz
# postgres role + db (idempotent; regenerate password each run into the env file)
DBPASS=$(openssl rand -hex 16)
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='oasis'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE ROLE oasis LOGIN PASSWORD '$DBPASS';"
sudo -u postgres psql -qc "ALTER ROLE oasis PASSWORD '$DBPASS';"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='oasis'" | grep -q 1 \
  || sudo -u postgres psql -qc "CREATE DATABASE oasis OWNER oasis;"
cat > /opt/oasis/api.env <<EOF
APP_ENV=development
HTTP_ADDR=:8090
DATABASE_URL=postgres://oasis:$DBPASS@127.0.0.1:5432/oasis?sslmode=disable
AUTO_MIGRATE=true
KYC_AUTO_APPROVE=true
STORAGE_DRIVER=local
STORAGE_DIR=/opt/oasis/storage
CORS_ALLOW_ORIGIN=http://$PUBLIC_IP:3200
EOF
chmod 600 /opt/oasis/api.env
# systemd: api
cat > /etc/systemd/system/oasis-api.service <<UNIT
[Unit]
Description=Oasis API
After=network.target postgresql.service
Requires=postgresql.service
[Service]
Type=simple
WorkingDirectory=/opt/oasis
EnvironmentFile=/opt/oasis/api.env
ExecStart=/opt/oasis/api
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload && systemctl enable --now oasis-api
echo "==> [5/6] build frontend"
cd /opt/oasis/frontend
npm config set registry https://registry.npmmirror.com >/dev/null 2>&1 || true
( npm ci --no-audit --no-fund || npm install --no-audit --no-fund ) >/tmp/fe-build.log 2>&1
NEXT_OUTPUT_STANDALONE=0 NEXT_PUBLIC_API_BASE_URL=http://$PUBLIC_IP:8090/api/v1 \
  NODE_OPTIONS=--max-old-space-size=3072 npm run build >>/tmp/fe-build.log 2>&1
cat > /etc/systemd/system/oasis-web.service <<UNIT
[Unit]
Description=Oasis Web
After=network.target oasis-api.service
[Service]
Type=simple
WorkingDirectory=/opt/oasis/frontend
Environment=NODE_ENV=production
ExecStart=/opt/oasis/frontend/node_modules/.bin/next start -p 3200 -H 0.0.0.0
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload && systemctl enable --now oasis-web
sleep 6
echo "==> [6/6] seed 8 research datasets"
set -a; . /opt/oasis/api.env; set +a
/opt/oasis/seedpublic || true
echo "--- health ---"
echo "api:  $(systemctl is-active oasis-api)  $(curl -sS --max-time 5 http://127.0.0.1:8090/healthz)"
echo "web:  $(systemctl is-active oasis-web)  HTTP $(curl -sS --max-time 6 -o /dev/null -w '%{http_code}' http://127.0.0.1:3200/datasets)"
REMOTE

echo ""
echo "==> DONE. Open:  http://$IP:3200/datasets   (API: http://$IP:8090/healthz)"
echo "    Make sure the firewall/security group opens inbound TCP 22, 3200, 8090."
