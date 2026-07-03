#!/usr/bin/env bash
# Deploy Lumen + Lumen Science integration onto the Oasis demo VPS.
# Run from repo root on your Mac (needs SSH key ~/.ssh/oasis_deploy).
#
#   bash scripts/deploy-lumen-vps.sh
#
# What it does:
#   1. cross-builds lumen for linux/amd64
#   2. rsyncs marketplace frontend changes + lumen binary
#   3. updates nginx to proxy /lumen/* and /lumen-science/*
#   4. installs systemd units for lumen-serve (:8787) and lumen-science (:18990)
#   5. rebuilds Next.js frontend and restarts services
set -euo pipefail

VPS_HOST="${VPS_HOST:-118.31.47.129}"
VPS_USER="${VPS_USER:-root}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/oasis_deploy}"
LUMEN_SRC="${LUMEN_SRC:-$HOME/lumen}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

SSH=(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "${VPS_USER}@${VPS_HOST}")
RSYNC=(rsync -az -e "ssh -i $SSH_KEY -o StrictHostKeyChecking=no")

echo "▸ cross-build lumen (linux/amd64)…"
mkdir -p "$LUMEN_SRC/bin"
(
  cd "$LUMEN_SRC"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/lumen-linux-amd64 ./cmd/lumen
)

echo "▸ sync frontend + lumen binary…"
"${RSYNC[@]}" "$REPO_ROOT/frontend/components/Nav.tsx" \
  "${VPS_USER}@${VPS_HOST}:/opt/oasis/frontend/components/Nav.tsx"
"${RSYNC[@]}" "$REPO_ROOT/frontend/next.config.mjs" \
  "${VPS_USER}@${VPS_HOST}:/opt/oasis/frontend/next.config.mjs"

"${RSYNC[@]}" "$LUMEN_SRC/bin/lumen-linux-amd64" \
  "${VPS_USER}@${VPS_HOST}:/usr/local/bin/lumen"
"${SSH[@]}" chmod +x /usr/local/bin/lumen

echo "▸ install systemd units + nginx routes…"
"${SSH[@]}" bash -s <<'REMOTE'
set -euo pipefail

cat > /etc/systemd/system/lumen-serve.service <<'UNIT'
[Unit]
Description=Lumen programming agent UI (Oasis /lumen proxy target)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lumen serve --addr 127.0.0.1:8787
Restart=always
RestartSec=3
User=root
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
UNIT

cat > /etc/systemd/system/lumen-science.service <<'UNIT'
[Unit]
Description=Lumen Science control panel (Oasis /lumen-science proxy target)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lumen science gui --addr 127.0.0.1:18990 --no-browser
Restart=always
RestartSec=3
User=root
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
UNIT

cat > /etc/nginx/sites-available/oasis <<'NGINX'
server {
  listen 8080;
  server_name _;
  client_max_body_size 64m;
  absolute_redirect off;
  port_in_redirect off;

  # Cloudflare terminates TLS; origin sees HTTP on :8080 — preserve public https URLs.
  set $public_scheme https;

  location /api/    { proxy_pass http://127.0.0.1:8090; proxy_set_header Host $host; proxy_set_header X-Forwarded-For $remote_addr; proxy_set_header X-Forwarded-Proto $public_scheme; }
  location /healthz { proxy_pass http://127.0.0.1:8090; proxy_set_header X-Forwarded-Proto $public_scheme; }
  location /docs    { proxy_pass http://127.0.0.1:8090; proxy_set_header X-Forwarded-Proto $public_scheme; }

  # Lumen programming agent — strip /lumen/ prefix before upstream
  location = /lumen { return 301 $public_scheme://$host/lumen/; }
  location /lumen/ {
    proxy_pass http://127.0.0.1:8787/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $public_scheme;
    proxy_buffering off;
    proxy_read_timeout 300s;
  }

  # Lumen Science — strip /lumen-science/ prefix before upstream
  location = /lumen-science { return 301 $public_scheme://$host/lumen-science/$is_args$args; }
  location /lumen-science/ {
    proxy_pass http://127.0.0.1:18990/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $public_scheme;
    proxy_buffering off;
    proxy_read_timeout 300s;
  }

  location / {
    proxy_pass http://127.0.0.1:3200;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $public_scheme;
  }
}
NGINX

mkdir -p /root/.config/lumen
cat > /root/.config/lumen/lumen.toml <<'TOML'
default_model = "deepseek-chat"

[[providers]]
name = "deepseek"
kind = "openai"
base_url = "https://api.deepseek.com/v1"
model = "deepseek-chat"
api_key_env = "DEEPSEEK_API_KEY"

[agent]
max_steps = 30
temperature = 0.7
TOML

systemctl daemon-reload
systemctl enable --now lumen-serve lumen-science
nginx -t && systemctl reload nginx
REMOTE

echo "▸ rebuild frontend on VPS…"
"${SSH[@]}" bash -s <<'REMOTE'
set -euo pipefail
cd /opt/oasis/frontend
npm run build
systemctl restart oasis-web
REMOTE

echo "▸ smoke test…"
sleep 4
curl -fsS "https://demo.oasisdata2026.xyz/lumen/" | head -c 200 && echo " … /lumen ok"
curl -fsS "https://demo.oasisdata2026.xyz/lumen-science/?embed=1" | head -c 200 && echo " … /lumen-science ok"

echo
echo "════════════════════════════════════════════════════════════"
echo "  Lumen integration LIVE on https://demo.oasisdata2026.xyz"
echo "  Nav: Lumen · Lumen Science"
echo "  Paths: /lumen/ · /lumen-science/?embed=1&oasis=1"
echo "════════════════════════════════════════════════════════════"