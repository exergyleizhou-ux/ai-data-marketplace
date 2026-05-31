// Package platform holds cross-cutting infrastructure shared by all modules:
// database access, Redis, the async task queue (Asynq), structured logging,
// the unified HTTP response/error envelope, auth/RBAC middleware, rate
// limiting and the append-only audit log.
//
// These are horizontal concerns (SEC in the architecture diagram), not a
// business module. Subpackages are added incrementally:
//   - platform/httpx   unified response + error codes (PR-03)
//   - platform/db      Postgres pool + migrations wiring (PR-02)
//   - platform/redis   client + lock + limiter (PR-06)
//   - platform/queue   Asynq client/worker (PR-09)
//   - platform/audit   append-only audit log (PR-18 / used throughout)
package platform
