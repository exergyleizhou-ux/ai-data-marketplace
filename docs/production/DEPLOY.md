# Production deployment candidate

This procedure creates a deployable candidate. It does not authorize a public
deployment, a push, or a merge to `main`.

## Preconditions

- Build and scan immutable Oasis backend/frontend and Lumen images. Set `TAG`
  and `LUMEN_IMAGE` to a git SHA or digest; never use `latest`.
- Copy `.env.production.example` to the host secret store. Do not copy a filled
  env file into an image or repository. Use distinct JWT, runtime-ingest, PII,
  database, storage and metrics secrets.
- Record `PREVIOUS_TAG`, take a restorable Postgres snapshot, verify object-store
  versioning/lifecycle, and record the current schema version.
- Permit inbound traffic only to Caddy on 80/443. Backend, frontend, Postgres,
  Redis, object storage, and all Lumen ports remain private.

## Release

From `deploy/`:

```bash
export ENV_FILE="$PWD/.env"
test -n "$TAG" && test "$TAG" != latest
docker compose --env-file .env -f docker-compose.prod.yml pull
docker compose --env-file .env -f docker-compose.prod.yml run --rm migrate
docker compose --env-file .env -f docker-compose.prod.yml up -d --no-deps backend frontend
docker compose --env-file .env -f docker-compose.prod.yml up -d caddy
curl -fsS "https://$DOMAIN/healthz"
curl -fsS "https://$DOMAIN/readyz"
curl -fsS "https://$DOMAIN/"
```

Migration is intentionally one-shot and must finish before application rollout.
Application replicas run with `AUTO_MIGRATE=false`, avoiding migration races.
`healthz` proves the process is alive; `readyz` proves dependencies are usable.

## Proxy contract

Caddy is the only public service. It preserves Host and normal proxy headers;
Cookie and Origin pass through unchanged. Workbench SSE routes disable response
buffering and have a ten-minute upstream read timeout. The proxy never accepts
an arbitrary target URL.

## Acceptance

Check readiness, frontend HTML, authentication/session logout, Code and Lab Run
creation/events/cancel/approval/artifact/evidence, and one reconnecting SSE flow.
Confirm application logs have `request_id`, bounded `run_id`, and irreversible
`user_hash`, and contain no prompt, token, API key, Cookie, or Authorization.
Run the frontend E2E suite against the candidate origin before promotion.
