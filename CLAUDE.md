# AI Data Marketplace

A data marketplace: list datasets, order/pay, deliver, with quality checks, search, and
privacy-preserving compute (C2D). Go backend + Next.js frontend + Postgres + Redis.

## Stack

- **Backend**: Go 1.25 (`GOTOOLCHAIN=auto` auto-downloads on first build), Gin, pgx/v5,
  go-redis/v9, golang-migrate. → use `golang-patterns`, `golang-testing`, `postgres-patterns`,
  `redis-patterns`, `api-design`, `database-migrations`, `security-review`.
- **Frontend**: Next.js 14 **App Router** (`frontend/app/`), React 18, TypeScript, Tailwind.
  → use `react-patterns`, `react-testing`, `react-performance`. **Not Vite** — ignore vite-patterns.
- **Infra**: Docker Compose (`make up`), Postgres 16, Redis. → `docker-patterns`, `e2e-testing`.

## Architecture

Modular monolith. Each domain module under `backend/internal/modules/<name>/` owns its full
vertical slice with a consistent layering:

```
handler.go   → HTTP (Gin), request/response, validation
service.go   → business logic, orchestration
repo.go      → data access (pgx)
model.go     → domain types
router.go    → route registration
middleware.go, *_test.go
```

Modules: `auth, dataset, delivery, order, payment, quality, search`.
Shared: `internal/config`, `internal/platform`, `internal/server`. Entry: `cmd/api`.

Keep the layering clean: handlers never touch the DB directly; services depend on repo
interfaces. For deeper boundary work see `hexagonal-architecture` (the project uses the
lighter handler/service/repo split, not full ports/adapters — don't over-engineer it).

## Commands

```bash
# Always prepend PATH first (toolchain is hand-installed, no brew/docker by default):
export PATH="$HOME/.local/bin:$HOME/sdk/pg/bin:$HOME/.bun/bin:$PATH"

make up                       # full local stack (pg, redis, backend, frontend)
make backend-run              # run API locally
make backend-test             # Go tests
make backend-tidy             # go mod tidy → go.sum
make migrate-up / migrate-down
make migrate-create name=add_foo
make frontend-dev / frontend-build
```

### Docker-less e2e (validated pattern)

No `psql`/`createdb` available — pg is the zonky embedded binary (`initdb`/`pg_ctl`/`postgres` only):
`initdb -U app --auth=trust` → start pg on port 55432 with `-k /tmp` → use default `postgres` DB →
run api binary with `AUTO_MIGRATE=true` → curl the API. npm installs may ECONNRESET; retry with
`--fetch-retries`.

## Conventions / gotchas

- Migrations: golang-migrate, paired up/down in `backend/migrations/`. Follow `database-migrations`
  safety rules (nullable-or-default columns, `CREATE INDEX CONCURRENTLY`, no DDL+DML in one file).
- DB access via pgx/v5 + pgxpool — see `postgres-patterns` (pool config, advisory locks, JSONB).
- Auth module already has KYC + token + middleware; reuse, don't reinvent.
- Before claiming a change works: run `make backend-test` (and the e2e pattern for migration/SQL/HTTP
  changes). Evidence before "done" — `superpowers:verification-before-completion`.
- Money/payment & access-control changes are high-risk → run `security-review` and `/cso`.

## C2D / privacy compute

Privacy-preserving compute stack is built end-to-end (engine, API, frontend, real Docker runner,
DP, TEE attestation design). To go live: `COMPUTE_RUNNER=docker` + register the digest-pinned algo
images per `docs/部署-C2D算法镜像与生产Runner.md`.
