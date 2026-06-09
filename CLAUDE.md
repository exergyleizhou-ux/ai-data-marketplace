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
- **"enqueue-then-mark-ready" race** (cost a CI flake): if you enqueue async work and only THEN flip
  the parent to a "ready/advanceable" status, fast workers can finish before the flip and their
  completion callbacks no-op (parent not ready yet) → parent hangs forever. Fix: after flipping to
  ready, **explicitly run the advance check once** (idempotent), and guard early callbacks on the
  ready status so they don't make a premature partial decision. See `SubmitFederatedJob`→`tryAdvanceFederated`.
- Timestamps on DTOs are `string` (scanned via `::text`), not `time.Time` — match the existing style.
- Run `gofmt -w` on touched files before committing (struct-field alignment shifts after edits).
- macOS has no `tac`/`timeout`/`migrate`/`brew`; use `tail -r`, Go context timeouts, embedded migrations.
- **Smart/curly quotes in `.tsx`**: large Write/Edit blocks can introduce `“` `”` (U+201C/U+201D)
  instead of straight `"`. They're **invalid as JSX attribute or string delimiters** and tsc fails with
  cryptic `TS1127 Invalid character` / `TS1005 ',' expected`. Fix: replace curly→straight quotes
  (`python3 -c "...replace('“','\"')..."`), but keep curly quotes that are intentional *inside*
  visible string content — escape the conflict by using curly quotes only in the text, straight as delimiters.
- **Integration tests must use `db.RunMigrations(dsn)`, never raw `CREATE TABLE`**: a fake schema
  (e.g. `buyer_id TEXT` vs production `UUID REFERENCES users`) masks FK / type bugs. PR #74
  `timeseries_test.go` used bare `CREATE TABLE` and passed CI because the SQL path didn't depend on
  FK, but a later migration or query change would break silently.
- **New modules must ship with `_test.go`**: PR #75 `notification` and `verify` modules landed with
  0 tests. IDOR (`WHERE id=$1 AND user_id=$2`) and `ON CONFLICT DO NOTHING` idempotency had no
  regression coverage. From now on, every `repo.go`/`service.go` file must have a corresponding
  `_test.go`.
- **Notification emit must use `nil` guard + swallow errors**: `if s.notifier != nil { _ = s.notifier.NotifyUser(...) }`.
  Business flows must never block on notification failure. Pattern at `order/service.go` MarkPaid /
  MarkSettled / Dispute.
- **Background goroutine scanners must NOT read from work-queue channels as a stop signal**:
  a `chan qualityJob` carries real tasks — reading from it in a retry scanner steals work from the
  worker pool. Use a separate `context.Context` + `cancel()` for graceful shutdown of periodic
  scanners. PR-J `qualityRetryLoop` initially read from `s.qCh` (the work queue), causing silent
  job loss.
- **Exponential backoff must be parameterised on `attempts`**: a hardcoded constant (e.g. always
  30 s) defeats the retry purpose and can amplify sidecar-5 xx storms. PR-J `computeRetryBackoff`
  implements 30 s → 60 s → 120 s as a pure function, keeping the worker stateless and testable.
- **Zip bundle: validate ALL orders before writing the first zip byte**: `BundleOrders` performs
  a complete pre-flight (ownership, status, product type, key resolution) before `zip.NewWriter`
  writes anything. This guarantees a rejected request leaves the HTTP response body empty rather
  than a corrupt zip. PR-K.
- **Streaming-response handlers must complete all validation BEFORE setting HTTP status**:
  `c.Status(200)` + `c.Header("Content-Type", "application/zip")` called before `BundlePreflight`
  caused preflight failures (foreign order→403, non-settled→409, compute→400) to return `200 OK +
  Content-Type: application/zip + empty body`. Fix: split into Preflight (returns json error via
  `fail(c, err)`) + Stream (sets zip headers after preflight passes). PR-K fix.
- **Watchlist Add initialises `last_notified_version_id` from `datasets.current_version_id`**: the
  `INSERT … SELECT $1, $2, current_version_id FROM datasets WHERE id = $2` pattern ensures new
  watchers only receive notifications for future versions, avoiding a spurious notification on the
  current version. PR-L.
- **QA AnswerQuestion must check `answererID == sellerID` (IDOR)**: `Service.AnswerQuestion`
  calls `ds.SellerOf(ctx, q.DatasetID)` and rejects with `ErrNotSeller` if the answerer is not
  the dataset's seller. Same pattern as `notification.MarkRead` `WHERE id=$1 AND user_id=$2`.
  PR-O.
- **QA ListByDataset uses `JOIN users` for asker_name with `SUBSTRING(account, 1, 8)`**: the
  account prefix (not full email) is exposed as the public display name. `LEFT JOIN answers`
  populates `Question.Answer` only when present. PR-O.
- **Withdrawal Transition must guard against transitions FROM `completed`**: `completed` is a terminal
  state in the `pending→approved→completed` state machine. From `completed`, ALL transitions
  must return `ErrBadTransition`. This guard sits in `pgRepo.Transition` before the optimistic-lock
  UPDATE. PR-P.
- **Anomaly unique index must use only IMMUTABLE expressions**: `DATE(col)` / `col::date`
  are STABLE, not IMMUTABLE, so they cannot appear in a real unique index. Use a
  partial unique index `WHERE status='open'` with `COALESCE(actor_id::text,'')` instead. PR-Q.
- **Anomaly scanner must NOT read from work-queue channels as a stop signal**: same lesson
  as PR-J. The anomaly scanner uses `ctx.Done()` + `ticker.C` only. PR-Q.
- **Integration tests must NEVER `DROP TABLE … CASCADE`**: even with `IF EXISTS`, dropping a
  production table destroys schema for every other test in a `-p 1` run.  Use `TRUNCATE TABLE`
  (idempotent, schema-preserving) to clean rows between tests, never DROP.  PR #74
  timeseries_test.go originally did this and PR #81 had to add a defensive skip guard.  PR-M fix.
- **`seedUser` for integration tests must use crypto/rand suffix + ON CONFLICT DO UPDATE**:
  `time.Now().UnixNano()` collides under nano-clock resolution or parallel test runs.  The
  combination guarantees zero `users.account` UNIQUE conflicts even at high call rates.  PR-N fix.

## C2D / privacy compute (信任阶梯 L0→L3)

- **L1** data sandbox (`runner_docker.go`, `--network=none`) — verified on real Docker.
- **L2** confidential computing (`runner_tee.go`, attester + remote attestation; MockAttester until TEE cloud).
- **L3** data-stays-home: **federated learning built** (`compute/aggregator.go` real FedAvg,
  `compute/federated.go` orchestration + `min_participants` fault tolerance). Sub-jobs run in each
  dataset's sandbox; only model params aggregate; partials are internal-only. `POST /compute/federated-jobs`.
  Next: P4-b (real fed-logreg image + secure aggregation), P4-c (MPC/PSI). Design: `docs/设计文档-P4-*`.
- To go live: `COMPUTE_RUNNER=docker` (or `tee`) + register digest-pinned algos per
  `docs/部署-C2D算法镜像与生产Runner.md`. Specs/plans for recent work under `docs/superpowers/`.
