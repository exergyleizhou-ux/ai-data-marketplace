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

## Phase 4 production-ingest reacceptance (2026-07-13)

- A dedicated, fail-closed Lumen machine contract now calls the production
  persistence methods for run creation/CAS transitions, ordered events, usage,
  approvals and artifacts. It uses `WORKBENCH_RUNTIME_INGEST_SECRET`, which is
  distinct from user JWT signing material, compared in constant time, required
  in production and rejected when absent or shorter than 32 bytes.
- Artifact ingestion writes through the server's existing configured local/S3
  `storage.Storage`, constructs owner/run/artifact keys server-side, enforces a
  byte limit, records the object digest/size and removes bytes on metadata
  failure. Replays return existing owner-scoped metadata without rewriting.
- Migration `000037_workbench_runtime_execution` atomically binds approval
  consumption and execution outcomes to a unique execution ID and checked
  lifecycle. Approvals carry run/step/tool-call identity; artifacts carry
  step/tool-call/model/input references with nullable-safe uniqueness.
- The full authenticated ingest integration test exercises anonymous rejection,
  idempotent run/event/usage/artifact calls, real local object storage, approval
  decision, args-hash/version-bound consumption and executed completion.
- `TestComputePSIIntegration` now migrates and runs asynchronous workers in a
  unique temporary Postgres schema, rather than racing other packages' shared
  fixtures; three consecutive real-Postgres runs pass.

## Phase 5 unified Workbench summary (2026-07-13)

- The bridge parser remains strictly compatible with the exact Lab v1 shape and
  now accepts the exact v2 Code/Lab discriminated union. Unknown fields,
  sensitive payload additions, invalid identifiers, statuses and verification
  values fail closed. Both surfaces share one loader, timeline and summary UI.
- The authenticated same-origin `/api/workbench/status` handler probes Oasis,
  Code, Lab and the Science bridge and reports configuration readiness for the
  workspace store, model provider, object storage and compute runner. It emits
  only bounded `ok|degraded|down`, `reason_code` and `next_action` fields; raw
  upstream bodies, stack traces, keys and paths are never forwarded.
- The responsive Runtime panel presents terminal state, ordered event timeline,
  pending approval action, explicit verification state, artifact and evidence
  downloads, cancellation, and actionable degraded services. Terminal retries
  require a fresh user-entered prompt and create a new owner-scoped Run through
  the Lumen chat contract with `parent_run_id`; old prompts are never recovered.
- Mobile controls use a 390px-safe fixed panel and 44px minimum targets; forms,
  status/error announcements, labels, focus indicators and semantic timeline
  markup support keyboard and screen-reader use.

### Verification evidence

- `npm test`: PASS, 25 files / 91 tests.
- `npm run typecheck`: PASS.
- `npm run lint`: PASS, 0 errors / 37 pre-existing warnings; the warning in the
  touched workspace page was removed, reducing the prior baseline by one.
- `npm run build`: PASS, including dynamic `/api/workbench/status` output.
- `git diff --check`: PASS.

### Phase 5 accessibility and security reacceptance

- Runtime approvals are now fetched from the owner-scoped Lumen review API and
  rendered as bounded risk/effect/cost cards with explicit approve/reject
  actions. The Workbench does not expose command, args, prompt, path or target
  content. Code/Lab artifact links use owned run and opaque artifact IDs.
- The Runtime summary is a modal dialog with initial focus, focus containment,
  Escape close and focus return. Workbench tabs implement roving tab stops,
  ArrowLeft/ArrowRight/Home/End navigation and labelled tabpanel semantics;
  session loading and failure states are announced.
- The embedded runtime no longer combines `allow-scripts` with
  `allow-same-origin`. Its opaque `null` origin is accepted only together with
  the exact iframe `contentWindow`; the Lumen bridge still targets the explicit
  parent origin and never uses `*`.
- Workbench session creation now forwards the readable CSRF cookie as the
  required header to the same-origin BFF; the short-lived runtime JWT remains
  HttpOnly and is never returned to application JavaScript.

## Phase 7 reliability and security hardening (2026-07-13)

- A user whose authoritative `users.status` is not `active` can no longer mint
  a new Workbench token through an access token issued before the freeze. The
  independent Workbench JWT remains capped at five minutes by default (and at
  ten minutes by configuration validation), bounding already-issued sessions.
- Browser token requests are capped at 4 KiB and machine runtime payloads at
  1 MiB before JSON decoding/persistence. Invalid runtime credentials receive a
  generic response that does not echo credential material.
- Next responses now enforce a CSP that limits framing, connections, forms and
  active content to the same origin. `SAMEORIGIN` intentionally permits the
  same-origin Code/Lab Workbench iframe while cross-origin framing is rejected
  by both X-Frame-Options and `frame-ancestors 'self'`.
- Startup logs no longer include object-store endpoint/bucket/path or anomaly
  webhook URL fragments, because those values may contain infrastructure names,
  userinfo or signed path/query credentials.
- Vitest was upgraded from 2.1.9 to 3.2.7 and explicit Vite 6.4.3 was installed
  in isolated commit `f68c145`. Before and after the upgrade, all 25 frontend
  files / 94 tests, typecheck, lint (0 errors / 37 existing warnings) and the
  production build passed.
- `npm audit --omit=dev` reports zero high and zero critical vulnerabilities.
  Its two moderate findings are the same PostCSS advisory in Next's bundled
  dependency chain. npm offers only `--force` to downgrade Next to 9.3.3, which
  would remove current security fixes and break the application; this unsafe
  remediation is declined pending a patched stable Next release. The affected
  PostCSS stringify path is not fed attacker-authored CSS by Oasis.
- SQL review found all values in Workbench persistence passed as pgx bind
  parameters. Shared repository column-list concatenations are compile-time
  constants, not request data. Runtime persistence errors and HTTP responses do
  not include payloads, prompts, credentials or SQL text.
- With the hardening test included, frontend verification is 25 files / 95
  tests plus typecheck and production build. Backend `go test -race ./...`,
  `go vet ./...`, and `go build ./...` pass. The full race suite also passes
  against the real Docker Postgres, including Workbench repository/runtime
  ingest, server integration and asynchronous compute fixtures.
