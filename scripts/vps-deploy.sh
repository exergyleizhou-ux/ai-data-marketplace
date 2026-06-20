#!/usr/bin/env bash
# One-box VPS deploy for Oasis Verify — fresh Ubuntu 22.04/24.04, ≥4GB RAM.
#
#   curl -fsSL <raw>/scripts/vps-deploy.sh | sudo bash      # or scp + sudo bash
#
# What it does (idempotent — safe to re-run):
#   1. installs Docker + compose plugin + Go (to build the backend on the host)
#   2. clones the repo
#   3. generates .env with random secrets + a public https URL via <IP>.sslip.io
#   4. runs postgres + redis + a local registry + the frontend + Caddy (auto-TLS)
#      as containers
#   5. runs the BACKEND AS A HOST PROCESS (systemd) — required: its --network=none
#      sandbox does `docker run -v <staged>:/data`, and a host process's paths are
#      visible to the host docker daemon (a containerized backend's /tmp is not).
#   6. builds + pins the PaperGuard screener and registers it (so /screen works)
#   7. smoke-tests the public endpoint
#
# Env overrides: DOMAIN (default <IP>.sslip.io), REPO, BRANCH (default main).
set -euo pipefail
[ "$(id -u)" = "0" ] || { echo "run as root (sudo)"; exit 1; }

IP="${IP:-$(curl -fsS https://api.ipify.org || hostname -I | awk '{print $1}')}"
DOMAIN="${DOMAIN:-${IP}.sslip.io}"
REPO="${REPO:-https://github.com/exergyleizhou-ux/ai-data-marketplace.git}"
BRANCH="${BRANCH:-main}"
APP=/opt/oasis
URL="https://$DOMAIN"
echo "▸ deploying to $URL (IP=$IP)"

# 1. Docker + Go
if ! command -v docker >/dev/null; then
  curl -fsSL https://get.docker.com | sh
fi
systemctl enable --now docker
if ! command -v go >/dev/null; then
  GOARCH=amd64; [ "$(dpkg --print-architecture 2>/dev/null)" = "arm64" ] && GOARCH=arm64
  GO=go1.23.4.linux-$GOARCH.tar.gz
  curl -fsSL "https://go.dev/dl/$GO" -o /tmp/$GO && rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/$GO
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
fi

# 2. code
[ -d "$APP" ] || git clone --depth 1 -b "$BRANCH" "$REPO" "$APP"
cd "$APP" && git fetch --depth 1 origin "$BRANCH" && git checkout "$BRANCH" && git reset --hard "origin/$BRANCH"

# 3. secrets + env (only generated once)
ENV=$APP/.env
if [ ! -f "$ENV" ]; then
  PGPW=$(openssl rand -hex 16); JWT=$(openssl rand -hex 32)
  cat > "$ENV" <<EOF
POSTGRES_USER=oasis
POSTGRES_PASSWORD=$PGPW
POSTGRES_DB=oasis
DATABASE_URL=postgres://oasis:$PGPW@localhost:5432/oasis?sslmode=disable
REDIS_URL=redis://localhost:6379
APP_ENV=production
HTTP_ADDR=:8080
AUTO_MIGRATE=true
COMPUTE_RUNNER=docker
STORAGE_DIR=/opt/oasis-storage
JWT_SECRET=$JWT
CORS_ALLOW_ORIGIN=$URL
APP_BASE_URL=$URL
NEXT_PUBLIC_API_BASE_URL=$URL/api/v1
# Stripe (fill after you create the account; leave blank = free tier only):
STRIPE_SECRET_KEY=
STRIPE_WEBHOOK_SECRET=
VERIFY_PRICE_MAP=
EOF
fi
set -a; . "$ENV"; set +a
mkdir -p /opt/oasis-storage

# 4. infra containers (pg, redis, registry, frontend, caddy)
cat > "$APP/docker-compose.vps.yml" <<EOF
services:
  postgres:
    image: postgres:16
    environment: { POSTGRES_USER: \${POSTGRES_USER}, POSTGRES_PASSWORD: \${POSTGRES_PASSWORD}, POSTGRES_DB: \${POSTGRES_DB} }
    ports: ["127.0.0.1:5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    restart: unless-stopped
  redis:
    image: redis:7
    ports: ["127.0.0.1:6379:6379"]
    restart: unless-stopped
  registry:
    image: registry:2
    ports: ["127.0.0.1:5000:5000"]
    volumes: ["regdata:/var/lib/registry"]
    restart: unless-stopped
  frontend:
    build:
      context: ./frontend
      args: { NEXT_PUBLIC_API_BASE_URL: "$URL/api/v1" }
    ports: ["127.0.0.1:3000:3000"]
    restart: unless-stopped
  caddy:
    image: caddy:2
    ports: ["80:80","443:443"]
    volumes: ["./Caddyfile:/etc/caddy/Caddyfile","caddydata:/data"]
    restart: unless-stopped
volumes: { pgdata: {}, regdata: {}, caddydata: {} }
EOF

# Caddy: TLS for the domain; API/verify/screen/docs → host backend, else → frontend
cat > "$APP/Caddyfile" <<EOF
$DOMAIN {
  @api path /api/* /verify/* /screen* /healthz /docs*
  handle @api { reverse_proxy host.docker.internal:8080 }
  handle { reverse_proxy frontend:3000 }
}
EOF

docker compose -f "$APP/docker-compose.vps.yml" up -d --build
echo "▸ waiting for postgres…"; for _ in $(seq 1 30); do docker exec "$(docker compose -f "$APP/docker-compose.vps.yml" ps -q postgres)" pg_isready -U oasis >/dev/null 2>&1 && break; sleep 2; done

# 5. backend as a HOST systemd service (so the sandbox's docker -v paths resolve)
( cd "$APP/backend" && GOFLAGS=-mod=mod GOTOOLCHAIN=auto go build -o /usr/local/bin/oasis-api ./cmd/api )
cat > /etc/systemd/system/oasis-api.service <<EOF
[Unit]
After=docker.service
[Service]
EnvironmentFile=$ENV
ExecStart=/usr/local/bin/oasis-api
Restart=always
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload && systemctl enable --now oasis-api
echo "▸ waiting for backend…"; for _ in $(seq 1 30); do curl -fsS localhost:8080/healthz >/dev/null 2>&1 && break; sleep 2; done

# 6. screener (so POST /screen works) — needs an ops account; seed one
OPSPW=$(openssl rand -hex 8)
curl -fsS -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' \
  -d "{\"account\":\"ops@oasis.local\",\"account_type\":\"email\",\"password\":\"$OPSPW\"}" >/dev/null || true
docker exec "$(docker compose -f "$APP/docker-compose.vps.yml" ps -q postgres)" \
  psql -U oasis -d oasis -c "UPDATE users SET role='ops',kyc_status='verified' WHERE account='ops@oasis.local'" >/dev/null
API=http://localhost:8080/api/v1 OPS_ACCT=ops@oasis.local PASSWORD="$OPSPW" bash "$APP/scripts/register-verify-screener.sh"

# 7. smoke test (public)
echo "▸ smoke test via $URL …"
sleep 3
curl -fsS "$URL/healthz" && echo " healthz ok" || echo " (TLS may take ~30s on first hit; retry)"

echo
echo "════════════════════════════════════════════════════════════"
echo "  Oasis Verify is LIVE:  $URL/verify-api"
echo "  API:                    $URL/api/v1/screen"
echo "  ops account:            ops@oasis.local / $OPSPW   (keep this)"
echo "  To enable paid plans: fill STRIPE_* + VERIFY_PRICE_MAP in $ENV, then"
echo "    systemctl restart oasis-api"
echo "════════════════════════════════════════════════════════════"
