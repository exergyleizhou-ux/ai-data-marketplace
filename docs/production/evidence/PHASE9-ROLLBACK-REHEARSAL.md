# Phase 9 image rollback rehearsal

Date: 2026-07-13 UTC

Command:

```bash
COMPOSE_PROJECT_NAME=oasis-phase9_final \
  EVIDENCE_DIR="$PWD/work/phase9-evidence" \
  PREVIOUS_REF=91a9187^ \
  bash scripts/phase9-rehearsal.sh
```

Candidate source revision: `0cf7e06d6a12b7d5759dfc4a7760c3c4e11f1e88`
Actual previous application revision: `29e9e02c4d0aaa2ec7bec70fbe5b411646458a8a`

The isolated candidate images built and started, the database reached current
schema, and health/readiness/frontend checks passed. The embedded migration
test executed (not skipped) and passed a complete up/down/up cycle. Backend and
frontend images were then built from the exact previous revision, substituted
for the running application without a schema down migration, and passed
readiness plus frontend read compatibility. The current candidate images were
restored and passed the same gates.

The transcript records the production invariants `APP_ENV=production`,
`AUTO_MIGRATE=false`, and `LUMEN_HOSTED=true`; the local containers themselves
use the deliberately isolated development credentials and database. Cleanup
previewed and removed only `oasis-phase9_final_*` volumes and
`oasis-phase9_final-*` images. The shared runtime PostgreSQL container remained
running and was not modified or removed.

The exact raw transcript is committed as
`work/phase9-evidence/rehearsal-20260713T005422Z.log`.
