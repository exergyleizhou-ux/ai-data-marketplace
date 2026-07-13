# Oasis and Lumen production runbook

## Signals and alerts

Scrape authenticated `/metrics` over the private network. Alert on:

| Signal | Warning | Critical / action |
| --- | --- | --- |
| HTTP 5xx ratio | >2% for 10m | >5% for 5m; stop rollout |
| readiness | one failure | 3 consecutive failures; remove replica |
| Run duration | p95 >10m | p95 >20m; inspect provider/queue |
| approval wait | p95 >5m | p95 >15m; page operator |
| tenant queue | >70% capacity | >90%; shed with 429/503 |
| provider failures | >5% by provider/model | >15%; disable affected allowlist entry |
| DB/storage errors | any sustained 5m | readiness failure or write loss; stop writes |

Use bounded labels only: route templates, outcome, provider and model allowlist
values. Never label metrics by user, run ID, prompt, object key or error string.

## Triage

1. Start with the synthetic request's `request_id`; correlate its `run_id` and
   `user_hash` without exposing account IDs.
2. Check `/healthz`, `/readyz`, Caddy/backend/Lumen status, Postgres connections,
   Redis and object storage independently.
3. For stuck Runs, inspect durable events and approval expiry before attempting
   cancellation. Do not mutate rows manually.
4. For provider failure, disable only the affected provider/model and preserve
   idempotency keys. For queue saturation, shed load rather than grow an
   unbounded queue.
5. For DB or storage failure, stop writes and follow `ROLLBACK.md`; reconcile
   metadata and object bytes before reopening.

## Secret incident

Revoke the leaked credential at its source, rotate only the affected trust
boundary, restart consumers, invalidate sessions when signing material changed,
and search logs/object metadata for exposure. Never paste secret values into an
incident ticket. The runtime-ingest secret is distinct from browser JWT signing
material so it can be rotated independently.

## Routine checks

Daily: readiness, backup completion, queue/provider/error dashboards. Weekly:
restore a Postgres snapshot and verify object retrieval. Each release: migration
rehearsal from the oldest supported schema, application rollback rehearsal, SSE
reconnect, session revocation, and evidence bundle checksum verification.

