#!/usr/bin/env bash
# Restore a marketplace dump produced by backup.sh into a target database.
#
# DESTRUCTIVE: drops and recreates objects in the target (pg_restore --clean).
# Refuses to run without an explicit --yes.
#
# Usage:
#   DATABASE_URL=postgres://... ./restore.sh --yes <dumpfile>
#
# Notes:
# - Restore into an EMPTY or throwaway database first when verifying
#   (see drill.sh); point the app at it only after verification.
# - audit_logs is protected by an append-only trigger; --clean drops the
#   table (allowed) rather than deleting rows (blocked), so restore works.
set -euo pipefail

if [ "${1:-}" != "--yes" ] || [ -z "${2:-}" ]; then
  echo "usage: DATABASE_URL=postgres://... $0 --yes <dumpfile>" >&2
  echo "refusing to run without explicit --yes (this drops objects in the target DB)" >&2
  exit 2
fi
: "${DATABASE_URL:?DATABASE_URL is required}"
DUMP="$2"
[ -r "$DUMP" ] || { echo "restore: cannot read $DUMP" >&2; exit 2; }

echo "restore: verifying dump TOC"
pg_restore --list "$DUMP" >/dev/null

echo "restore: restoring $DUMP"
# --clean --if-exists: idempotent over partial schemas. --no-owner/--no-privileges:
# target role may differ from the source. --exit-on-error: fail loudly, no
# silently-partial restores.
pg_restore --clean --if-exists --no-owner --no-privileges --exit-on-error \
  --dbname="$DATABASE_URL" "$DUMP"

echo "restore: done"
