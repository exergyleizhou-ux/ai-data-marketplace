# Oasis Production Finalization Current State

Verified on 2026-07-12 in `/Users/lei/Documents/Codex/2026-07-12/new-chat-2/work/oasis-shared-runtime`.

## Repository

- Branch: `feat/oasis-shared-runtime-workbench`
- HEAD before this document: `8a1eee268665c11da4497bd6e2657a9d6b500fed` (`feat(workbench): show durable Lab runtime state`)
- The worktree was clean before this document was created.
- The branch contains the three shared-runtime Workbench commits `fb4635b`, `98d0742`, and `8a1eee2` after baseline `f0eb050`.
- The original checkout at `/Users/lei/ai-data-marketplace-seed` contains exactly the five untracked plans/specifications named in the production handoff; they were read without modification.

## Verified baseline

- `npm test -- lib/workbench-runtime.test.ts components/WorkbenchRuntimePanel.test.tsx`: PASS (2 files, 10 tests).
- `npm run typecheck`: PASS.
- An earlier invocation with paths prefixed by `frontend/` selected no tests and exited 1; the corrected repository-relative invocation above passed.

## Confirmed APIs and implementation boundaries

- The backend composes modules under `/api/v1` and registers authentication through `auth.Register`.
- `auth.Middleware` verifies Bearer access tokens and exposes identity through `httpx.UserID` and `httpx.UserRole`; `auth.RequireRole` provides route-level role checks.
- Database migrations are embedded from paired SQL files by `backend/migrations/embed.go`; migration `000034_api_keys.up.sql` is present.
- Object storage has local-filesystem and S3-compatible implementations behind `storage.Storage`.
- The frontend currently persists Oasis access and refresh tokens in browser `localStorage` through `tokenStore`.
- The current Workbench bridge accepts version 1 `lumen.workbench.snapshot` messages for the Lab surface, validates the iframe origin and source, and reads existing Lab Run, event, artifact, and cancel endpoints.
- Reverse-proxy configuration routes `/api/lab/*` and `/lumen-lab/*` to the Lumen Lab service.
- At the Phase 0 baseline, no Oasis `/api/v1/workbench/token` endpoint or browser HttpOnly Workbench session implementation existed; subsequent phases implemented both.

## Phase 4 managed runtime state (2026-07-13)

- Migration `000036_workbench_runtime` adds tenant-bound runs, ordered events,
  complete approvals, artifacts and integer usage/cost accounting. Composite
  foreign keys preserve `(account_id, workspace_id)` across every child row;
  run and approval versions support compare-and-swap updates.
- The Workbench repository now provides owner-scoped run/event/approval/artifact
  access, idempotent `(run_id, seq)` event append and `(run_id, event_id)` usage
  recording. Approval decisions require the expected version, remain owner
  scoped, and reject expired or already-decided records.
- Authenticated Workbench APIs expose run timelines, approvals, decisions and
  artifact metadata/download. Downloads resolve database-owned object keys
  through the existing shared local/S3 storage interface; object keys are not
  returned to browsers or accepted from browser input.
- OpenAPI includes all managed-runtime routes plus the existing browser-session
  endpoints. When Workbench identity signing is not configured, the token route
  remains discoverable but fails closed with `503`.

### Verification evidence

- Temporary Docker Postgres: migration up, explicit down in reverse dependency
  order, schema version reset to 35, and embedded migration up to clean version
  36: PASS; all five runtime tables recreated.
- Postgres integration tests: cross-owner run/approval/artifact isolation,
  concurrent run CAS exactly-one-winner, duplicate event append, duplicate usage
  accounting, approval decision and safe object-key construction: PASS.
- `DATABASE_URL=... go test -race ./internal/modules/workbench ./internal/server`: PASS.
- `go vet ./...`: PASS.
- `go build ./...`: PASS.
