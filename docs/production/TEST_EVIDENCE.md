# Phase 10 test evidence

Candidate initially verified at `4016b89054aa3f802a5d6b1602a281a7fde14e5a`
on branch `feat/oasis-shared-runtime-workbench`, 2026-07-13 (Asia/Shanghai).
The documentation commit after these gates contains no runtime code.

## Backend

| Gate | Result |
| --- | --- |
| `gofmt -d` over every backend Go file | PASS, 0 diff lines |
| `go test -race ./...` | PASS, 31 packages + 10 no-test packages |
| `go vet ./...` | PASS, no diagnostics |
| `go build ./...` | PASS, no diagnostics |

The accepted exact race command completed with no failed package and no race
report. The Apple linker emitted its known malformed `LC_DYSYMTAB` warning for
cgo-linked test binaries; linking and all tests completed successfully.

## PostgreSQL migrations

Against a fresh isolated PostgreSQL 16 container, with the embedded migration
set ending at version 39:

```text
DATABASE_URL=<isolated-local-postgres> go test ./internal/e2e \
  -run '^TestE2E_MigrationRoundtrip$' -count=1 -v
--- PASS: TestE2E_MigrationRoundtrip (5.06s)
PASS
```

The test creates a fresh database, applies all migrations to 39, applies every
down migration, and applies every up migration again.

## Frontend

| Gate | Result |
| --- | --- |
| `npm test` | PASS, 25 files / 98 tests |
| `npm run typecheck` | PASS |
| `npm run lint` | PASS, 0 errors / 37 existing warnings |
| `npm run build` | PASS, 32 routes emitted |
| `npm run e2e` | PASS, 14/14 Chromium journeys in 5.4m |
| `npm audit --omit=dev` | 2 moderate, 0 high, 0 critical |

The browser suite used fresh isolated PostgreSQL 16 and real Oasis, Next and
Lumen processes. It covered registration/session/logout, purchase, security
headers, approval/malicious bridge rejection, artifact fail-closed behavior,
anonymous/authenticated Workbench state, Code verification/self-fix,
cross-tenant isolation, Lab provenance/artifact/evidence, 390px layout, durable
recovery and cancellation.

The audit findings are the same PostCSS advisory in Next's bundled dependency.
npm offers only `--force`, which would install Next 9.3.3 and break this stack.
That downgrade was declined and must be rechecked at promotion.

## Repository hygiene

- `git diff --check`: PASS after documentation finalization.
- The five original untracked planning inputs remained unmodified.
- No push, main merge or public deployment occurred.
