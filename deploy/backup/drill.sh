#!/usr/bin/env bash
# Backup→restore drill: proves a dump taken by backup.sh actually restores.
# Run it in CI (backup-drill job) or against any throwaway database.
#
# DESTRUCTIVE on the target DB (drops schema public). Never point it at prod.
#
# Env:
#   DATABASE_URL  required  throwaway database
#   REPO_ROOT     optional  repo checkout (default: two levels up from here)
#
# Steps: real migrations → seed rows → backup.sh → DROP SCHEMA → restore.sh →
# verify row counts AND that the audit_logs append-only trigger survived.
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$HERE/../.." && pwd)}"
export BACKUP_DIR="$(mktemp -d)"
export RETENTION_DAYS=0

echo "drill[1/6]: applying real migrations"
(cd "$REPO_ROOT/backend" && go run ./cmd/api --migrate)

echo "drill[2/6]: seeding"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q <<'SQL'
INSERT INTO users (account, account_type, password_hash, role)
VALUES ('drill@backup.test', 'email', 'x', 'buyer');
INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, detail)
SELECT id, 'drill.seed', 'user', id::text, '{}'::jsonb FROM users
WHERE account = 'drill@backup.test';
SQL

echo "drill[3/6]: backup"
"$HERE/backup.sh"
DUMP="$(ls -t "$BACKUP_DIR"/marketplace-*.dump | head -1)"

echo "drill[4/6]: simulating loss (DROP SCHEMA public)"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q \
  -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

echo "drill[5/6]: restore"
"$HERE/restore.sh" --yes "$DUMP"

echo "drill[6/6]: verifying"
USERS=$(psql "$DATABASE_URL" -tA -c "SELECT count(*) FROM users WHERE account='drill@backup.test'")
AUDITS=$(psql "$DATABASE_URL" -tA -c "SELECT count(*) FROM audit_logs WHERE action='drill.seed'")
[ "$USERS" = "1" ] || { echo "FAIL: users=$USERS, want 1" >&2; exit 1; }
[ "$AUDITS" = "1" ] || { echo "FAIL: audit rows=$AUDITS, want 1" >&2; exit 1; }

# The append-only trigger must survive the roundtrip: an UPDATE must FAIL.
if psql "$DATABASE_URL" -q -c "UPDATE audit_logs SET action='tampered' WHERE action='drill.seed'" 2>/dev/null; then
  echo "FAIL: audit_logs UPDATE succeeded — append-only trigger lost in restore" >&2
  exit 1
fi

echo "drill: PASS — restore verified (rows intact, audit trigger intact)"
