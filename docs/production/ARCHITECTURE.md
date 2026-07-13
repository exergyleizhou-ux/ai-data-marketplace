# Production architecture

## Trust boundary and request path

Caddy is the only intended public listener. It terminates TLS and forwards the
browser application to Next.js, Oasis APIs to the Go backend, and the bounded
Code/Lab routes to Lumen. PostgreSQL, Redis, object storage, the Go backend,
Next.js, and all Lumen services remain private or loopback-only in the
production Compose candidate.

The browser authenticates to Oasis with a Secure, HttpOnly, SameSite cookie.
Oasis creates an independent, short-lived Workbench identity JWT and stores it
in another HttpOnly cookie; application JavaScript never receives either token.
The same-origin Next BFF forwards that credential only to configured Lumen
services. Mutating browser-session operations require the readable CSRF cookie
and matching request header.

## Authoritative state

Lumen executes Code and Lab work. Oasis PostgreSQL is the durable control-plane
record for tenant-scoped Runs, ordered events, approvals, artifacts, usage,
quotas and quota leases. Lumen writes that state through a separate machine
ingest secret; browser credentials cannot use the ingest contract. Every
repository operation includes account and workspace ownership, and composite
foreign keys preserve that ownership across child records.

Artifacts are written through the configured `storage.Storage` implementation.
Object keys are constructed server-side from validated opaque identifiers and
are never accepted from, or returned to, the browser. Metadata is committed
only after bytes are stored; failed metadata/quota commits remove newly written
bytes.

## Lifecycle

Runs use compare-and-swap versions for state transitions. Events are ordered
and idempotent by `(run_id, seq)`. Approvals bind run, step, tool call, argument
hash and expected version to exactly one execution. Usage and cost accounting
are integer based and idempotent. Quota reservations use durable leases so a
crash cannot permanently consume capacity.

The `/workspace` shell embeds the shipped Lumen surface in an opaque-origin
iframe. A strict, versioned `postMessage` bridge contains only routing identity
and bounded status fields. Oasis validates the exact iframe `contentWindow`
and message shape before loading owner-scoped details. The same UI presents
Code and Lab timelines, approvals, cancellation, artifacts and evidence.

## Deployment topology

`deploy/docker-compose.prod.yml` runs migrations as a one-shot service before
applications. Application auto-migration is disabled. Images are immutable
tags/digests, and current and previous tags are both recorded for rollback.
Health proves process liveness; readiness proves dependency availability.
Deployment and rollback operations are defined in `DEPLOY.md`, `ROLLBACK.md`
and `RUNBOOK.md`.

## Observability

Structured request logs carry a request ID, bounded run ID and irreversible
user hash. They do not log bodies, prompts, cookies, authorization headers or
keys. Prometheus alerts use bounded labels. Cross-service investigation uses
request/run IDs rather than sensitive payloads.
