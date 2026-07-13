# Production security posture

Verified implementation state at the Phase 10 acceptance candidate.

## Controls in force

- Browser sessions and Workbench sessions are Secure, HttpOnly and SameSite;
  logout revokes the server session before clearing client auth state and
  expires the Workbench cookie immediately.
- Workbench identity tokens use independent signing material, default to five
  minutes, and cannot be configured above ten minutes. Token minting rechecks
  the authoritative user status and fails closed for frozen users.
- Runtime ingest uses a distinct secret of at least 32 bytes, constant-time
  comparison, bounded request bodies and generic authentication failures.
- Tenant ownership is enforced in SQL/repository boundaries for Runs, events,
  approvals, artifacts, usage and quotas. Cross-tenant integration and browser
  journeys cover the negative cases.
- Approval execution is version-, identity- and argument-hash-bound, with one
  execution ID and explicit terminal outcome.
- Artifact paths are server-generated; local/S3 storage failures cannot be
  reported as durable success, and cleanup is tested.
- CSP limits scripts, connections, forms and frames. `frame-ancestors 'self'`
  and `X-Frame-Options: SAMEORIGIN` reject cross-origin framing while allowing
  the intended same-origin Workbench. The iframe itself has an opaque origin.
- The bridge parser rejects unknown fields, invalid identifiers, sensitive
  payload additions and unsupported versions. Parent messages require the
  exact iframe window; child messages target an explicit parent origin.
- CSRF protection covers cookie-authenticated mutations. Proxy targets are
  fixed by route configuration and cannot be supplied by a request.
- SQL values use pgx bind parameters. Structured logs and HTTP errors omit
  credentials, SQL, prompts, commands, arguments, filesystem paths and raw
  upstream bodies.
- Production Compose exposes only Caddy publicly and requires separate JWT,
  ingest, PII, database, storage and metrics secrets.

## Dependency decision

`npm audit --omit=dev` is a release gate. Moderate findings, if any, are
recorded in `TEST_EVIDENCE.md` and `KNOWN_LIMITATIONS.md`; a forced downgrade is
not an acceptable remediation. High or critical production findings block the
candidate.

## Secret handling

No filled environment file belongs in Git or an image. Rotate a suspected
secret using the sequence in `RUNBOOK.md`, revoke active sessions where
applicable, and correlate the incident only with request/run IDs and hashed
user identity. Never paste a credential into evidence or logs.

## Residual boundary

This local acceptance proves code, images, migrations and an isolated staging
journey. It does not prove the public host's TLS, firewall, DNS, external object
store, backup restore, alert delivery or third-party provider credentials;
those remain explicit pre-promotion gates.
