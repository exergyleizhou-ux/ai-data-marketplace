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
CURRENT_REF=$(git -C "$ROOT" rev-parse HEAD)
PREVIOUS_REF=$(git -C "$ROOT" rev-parse "${PREVIOUS_REF:-HEAD^}")
TMP=$(mktemp -d "${TMPDIR:-/tmp}/oasis-phase9-rehearsal.XXXXXX")
cleanup() {
  echo "== scoped cleanup preview ($PROJECT only) =="
  docker volume ls --format '{{.Name}}' | grep -E "^${PROJECT}_" || true
  docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -E "^${PROJECT}-" || true
  "${compose[@]}" down --volumes --remove-orphans --rmi local
  docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -E "^${PROJECT}-" | while IFS= read -r image; do
    docker image rm "$image" || true
  done
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "phase9_project=$PROJECT"
echo "git_head=$CURRENT_REF"
echo "previous_git_head=$PREVIOUS_REF"
echo 'production_env=APP_ENV=production AUTO_MIGRATE=false LUMEN_HOSTED=true'
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

candidate_backend="${PROJECT}-backend:candidate-${CURRENT_REF}"
candidate_frontend="${PROJECT}-frontend:candidate-${CURRENT_REF}"
previous_backend="${PROJECT}-backend:previous-${PREVIOUS_REF}"
previous_frontend="${PROJECT}-frontend:previous-${PREVIOUS_REF}"
docker tag "${PROJECT}-backend:latest" "$candidate_backend"
docker tag "${PROJECT}-frontend:latest" "$candidate_frontend"

echo '== build actual previous application images =='
git -C "$ROOT" archive "$PREVIOUS_REF" | tar -x -C "$TMP"
docker build --build-arg "OCI_REVISION=$PREVIOUS_REF" --build-arg "OCI_VERSION=$PREVIOUS_REF" \
  -t "$previous_backend" "$TMP/backend"
docker build --build-arg 'NEXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1' \
  --build-arg "OCI_REVISION=$PREVIOUS_REF" --build-arg "OCI_VERSION=$PREVIOUS_REF" \
  -t "$previous_frontend" "$TMP/frontend"

echo '== migration up/down/up rehearsal on isolated database =='
(cd "$ROOT/backend" && \
  DATABASE_URL='postgres://app:app@127.0.0.1:5432/ai_data_marketplace?sslmode=disable' \
  OASIS_E2E=1 go test ./internal/e2e -run '^TestE2E_MigrationRoundtrip$' -count=1 -v)

echo '== application rollback to actual previous images, schema remains current =='
"${compose[@]}" stop backend frontend
docker tag "$previous_backend" "${PROJECT}-backend:latest"
docker tag "$previous_frontend" "${PROJECT}-frontend:latest"
"${compose[@]}" up -d --force-recreate --no-deps backend frontend
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

echo '== restore current candidate images =='
"${compose[@]}" stop backend frontend
docker tag "$candidate_backend" "${PROJECT}-backend:latest"
docker tag "$candidate_frontend" "${PROJECT}-frontend:latest"
"${compose[@]}" up -d --force-recreate --no-deps backend frontend
for _ in $(seq 1 30); do
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

echo "PASS evidence=$LOG"
