# AI Data Marketplace — agent guide

Data marketplace: list datasets, order/pay, deliver, with quality checks, search, and
privacy-preserving compute (C2D, "可用不可见"). Go backend + Next.js frontend + Postgres + Redis.

## Stack & layout

- **Backend**: Go 1.23+ (`GOTOOLCHAIN=auto`), Gin, pgx/v5, go-redis, golang-migrate (embedded).
  **The Go module lives in `backend/`** — run all `go` commands from there (`cd backend && go build ./...`).
- **Frontend**: Next.js 14 App Router (`frontend/app/`), React 18, TypeScript, Tailwind. Not Vite.
- Modular monolith: `backend/internal/modules/<name>/` (auth, dataset, delivery, order, payment,
  quality, search, compute), each a vertical slice `handler→service→repo` (+ `model.go`, `router.go`,
  `*_test.go`). Shared: `internal/{config,platform,server}`. Entry: `cmd/api`.

## Workflow (mandatory)

The default working tree may sit on an old branch — **always branch off `origin/main`** in a worktree:

```bash
git fetch origin
git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main
# ...work + verify...
git push -u origin feat/<name>
gh pr create --base main --title "..." --body "..."
gh pr checks <n> --watch          # 3 jobs: backend / frontend / sidecar
gh pr merge <n> --squash --delete-branch
git worktree remove ~/ai-data-marketplace-<name>
```

One worktree per task. Toolchain is hand-installed; prepend PATH:
`export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"`.

## Verify before PR (run from the module dir; cwd resets between shells)

```bash
cd backend && gofmt -l . && go build ./... && go vet ./...
# real-DB tests need an ephemeral Postgres (no Docker / no psql/createdb here):
T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
initdb -D "$T" -U postgres --auth=trust >/dev/null
pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" go test -race ./...
pg_ctl -D "$T" stop -m fast >/dev/null
cd ../frontend && npm run typecheck && npm run lint && npm run build
```

Migrations are **embedded** and applied via `db.RunMigrations(dsn)` (no `migrate` CLI; integration
tests call this path). Frontend `node_modules` isn't shared across branches (package-lock differs) —
`npm ci` per worktree; npm may ECONNRESET, retry with `--fetch-retries=5`.

## Conventions / gotchas (each cost a real debugging cycle)

- **JSONB `NOT NULL DEFAULT '{}'` columns**: an INSERT passing explicit `nil` still violates the
  constraint. `toJSONB(nil)` returns nil → pass `[]byte("{}")` instead. The DEFAULT only applies when
  the column is omitted from the INSERT.
- **`uuid[]` params**: pass a `[]string` with an explicit cast — `$N::uuid[]` (see `UpsertOffer`,
  `CreateFederatedJob`). Scan back via `dataset_ids::text[]` into `[]string`.
- **Optimistic state machine**: status changes are `UPDATE ... WHERE status=$from RETURNING ...`;
  0 rows ⇒ `ErrBadTransition`. Concurrency-safe; use it for idempotent coordination.
- Timestamps on DTOs are `string` (scanned via `::text`), not `time.Time` — match the existing style.
- Run `gofmt -w` on touched files before committing (struct-field alignment shifts after edits).
- macOS has no `tac`/`timeout`/`migrate`/`brew`; use `tail -r`, Go context timeouts, embedded migrations.

## C2D / privacy compute (信任阶梯 L0→L3)

- **L1** data sandbox (`runner_docker.go`, `--network=none`) — verified on real Docker.
- **L2** confidential computing (`runner_tee.go`, attester + remote attestation; MockAttester until TEE cloud).
- **L3** data-stays-home: **federated learning built** (`compute/aggregator.go` real FedAvg,
  `compute/federated.go` orchestration + `min_participants` fault tolerance). Sub-jobs run in each
  dataset's sandbox; only model params aggregate; partials are internal-only. `POST /compute/federated-jobs`.
  Next: P4-b (real fed-logreg image + secure aggregation), P4-c (MPC/PSI). Design: `docs/设计文档-P4-*`.
- To go live: `COMPUTE_RUNNER=docker` (or `tee`) + register digest-pinned algos per
  `docs/部署-C2D算法镜像与生产Runner.md`. Specs/plans for recent work under `docs/superpowers/`.
