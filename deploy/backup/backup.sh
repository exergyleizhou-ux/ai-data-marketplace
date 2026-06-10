#!/usr/bin/env bash
# Logical backup of the marketplace database (pg_dump custom format).
#
# Env:
#   DATABASE_URL     required  postgres connection string
#   BACKUP_DIR       optional  output directory (default /backups)
#   RETENTION_DAYS   optional  delete local dumps older than N days (default 14)
#
# Output: $BACKUP_DIR/marketplace-YYYYmmdd-HHMMSS.dump (pg_dump -Fc, compressed,
# usable with pg_restore for full or per-table restore).
#
# Requires pg_dump (postgres client tools; the k8s CronJob uses postgres:16).
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$BACKUP_DIR/marketplace-$STAMP.dump"
TMP="$OUT.partial"

echo "backup: starting pg_dump -> $OUT"
pg_dump --format=custom --compress=6 --no-owner --no-privileges \
  --dbname="$DATABASE_URL" --file="$TMP"
mv "$TMP" "$OUT"   # atomic: a crash never leaves a truncated .dump

SIZE="$(du -h "$OUT" | cut -f1)"
echo "backup: done ($SIZE)"

# Retention: prune old local dumps (off-box copies are the upload step's job).
if [ "$RETENTION_DAYS" -gt 0 ] 2>/dev/null; then
  find "$BACKUP_DIR" -name 'marketplace-*.dump' -mtime "+$RETENTION_DAYS" -print -delete \
    | sed 's/^/backup: pruned /'
fi

# Integrity check: a listable TOC proves the dump is structurally sound.
pg_restore --list "$OUT" >/dev/null
echo "backup: integrity check passed (pg_restore --list)"
