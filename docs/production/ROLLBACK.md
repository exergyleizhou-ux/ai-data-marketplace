# Rollback and forward-fix

## Decision rule

Prefer an application rollback when the new schema remains backward compatible.
Prefer a forward-fix when new writes make a destructive schema down migration
unsafe. Never run a down migration merely to match an old image.

## Application rollback

1. Stop writes or place the service in maintenance mode.
2. Verify the old image supports the current schema and current Workbench rows.
3. Set `TAG=$PREVIOUS_TAG`; retain the current database schema.
4. Start backend/frontend, then require `/healthz`, `/readyz`, frontend, session,
   Run/event and artifact smoke checks before restoring traffic.

```bash
export TAG="$PREVIOUS_TAG"
docker compose --env-file .env -f deploy/docker-compose.prod.yml pull backend frontend
docker compose --env-file .env -f deploy/docker-compose.prod.yml up -d --no-deps backend frontend
```

## Schema rollback

Paired down migrations exist for development and disaster rehearsal. In
production, take a snapshot first, stop all writers, verify no data introduced
by the migration is needed, execute exactly one reviewed down step, and validate
the schema version and old application. Migrations 36-39 remove Workbench state;
therefore their production default is forward-fix, not down.

If readiness fails, keep Caddy serving maintenance/unavailable responses, retain
the database and object-store snapshot, capture request IDs, and escalate using
`RUNBOOK.md`. A rollback is complete only after synthetic Run and artifact tests
pass; container state alone is insufficient.

## Local replay command

Before promotion, replay the isolated application rollback and migration
roundtrip and retain its timestamped transcript:

```bash
COMPOSE_PROJECT_NAME=oasis-phase9_rehearsal \
  EVIDENCE_DIR="$PWD/work/phase9-evidence" \
  bash scripts/phase9-rehearsal.sh
```

This is an evidence rehearsal, not authorization to mutate a public host.
