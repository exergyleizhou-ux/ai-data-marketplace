#!/usr/bin/env bash
# Replayable, local-only Phase 9 install/migration/rollback evidence.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PROJECT=${COMPOSE_PROJECT_NAME:-oasis-phase9_rehearsal}
case "$PROJECT" in oasis-phase9_*) ;; *) echo "project must start oasis-phase9_: $PROJECT" >&2; exit 64;; esac

EVIDENCE_DIR=${EVIDENCE_DIR:-"$ROOT/work/phase9-evidence"}
mkdir -p "$EVIDENCE_DIR"
LOG="$EVIDENCE_DIR/rehearsal-$(date -u +%Y%m%dT%H%M%SZ).log"
exec > >(tee -a "$LOG") 2>&1

compose=(docker compose --project-name "$PROJECT" -f "$ROOT/docker-compose.yml")
cleanup() {
  echo "== scoped cleanup preview ($PROJECT only) =="
  docker volume ls --format '{{.Name}}' | grep -E "^${PROJECT}_" || true
  docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -E "^${PROJECT}-" || true
  "${compose[@]}" down --volumes --remove-orphans --rmi local
}
trap cleanup EXIT

echo "phase9_project=$PROJECT"
echo "git_head=$(git -C "$ROOT" rev-parse HEAD)"
docker version --format 'docker_server={{.Server.Version}}'
docker compose version

echo '== clean install / build =='
"${compose[@]}" up --build -d
for _ in $(seq 1 60); do
  curl -fsS http://127.0.0.1:8080/readyz >/dev/null && break
  sleep 2
done
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:3000/ >/dev/null && break
  sleep 2
done
curl -fsS http://127.0.0.1:3000/ >/dev/null

echo '== migration up/down/up rehearsal on isolated database =='
(cd "$ROOT/backend" && \
  DATABASE_URL='postgres://app:app@127.0.0.1:5432/ai_data_marketplace?sslmode=disable' \
  OASIS_E2E=1 go test ./internal/e2e -run '^TestE2E_MigrationRoundtrip$' -count=1 -v)

echo '== application rollback rehearsal =='
"${compose[@]}" stop backend frontend
"${compose[@]}" up -d --no-deps backend frontend
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:8080/readyz >/dev/null && break
  sleep 2
done
curl -fsS http://127.0.0.1:8080/readyz
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:3000/ >/dev/null && break
  sleep 2
done
curl -fsS http://127.0.0.1:3000/ >/dev/null

echo "PASS evidence=$LOG"
