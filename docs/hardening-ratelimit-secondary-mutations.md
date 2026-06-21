# Rate-limit hardening — secondary mutation endpoints

Supersedes the stale PR #124 (`fix/login-auth-context`, Ember cross-audit
follow-up). That branch was ~250 commits behind `main` and could not be
cleanly rebased: in the interim `main` independently rate-limited the
high-traffic / expensive routes (login, register, compute submit, payment,
order create, dataset create, file download, search, watchlist), so most of
#124 had become redundant or actively conflicting (e.g. watchlist is now
covered by a group-level `watch` 60/min limiter — strictly better than #124's
per-route `watch_add`/`watch_remove`).

Rather than force a Frankenstein merge, the **genuine net-new value** of #124 —
rate-limiting the *secondary* authenticated mutation endpoints that `main` had
not yet covered — was re-applied here on current `main`, following the existing
in-repo pattern (`middleware.RateLimit(limiter, RateLimitConfig{...})`).

## Coverage map

Already rate-limited on `main` (no change): `login`, `register`, `refresh`,
`logout`, `2fa_verify`, `password_reset_*`, `dataset_create`, `preview`,
`order_create`, `order_dispute`, `compute_job_submit`,
`compute_federated_submit`, `algo_request`, `compute_order`, `file_download`,
`payment_create`, `search`, `qa_ask`, `content_report`, `watch`,
`withdrawal_request`, `verify_*`.

Net-new in this change (18 endpoints):

| Module | Route | Name | Limit/min |
|---|---|---|---|
| anomaly | POST `/admin/anomalies/:id/acknowledge` | `anomaly_ack` | 20 |
| anomaly | POST `/admin/anomalies/:id/resolve` | `anomaly_resolve` | 20 |
| auth | POST `/auth/2fa/enroll` | `2fa_enroll` | 5 |
| auth | POST `/auth/2fa/verify-enrollment` | `2fa_verify_enroll` | 10 |
| auth | POST `/auth/2fa/disable` | `2fa_disable` | 5 |
| compliance | POST `/users/me/data-export` | `data_export` | 5 |
| compliance | POST `/users/me/account/deletion` | `account_deletion` | 3 |
| compliance | DELETE `/users/me/account/deletion` | `account_deletion_cancel` | 5 |
| compliance | POST `/admin/account-deletions/:id/execute` | `deletion_execute` | 10 |
| compute | POST `/compute/jobs/:id/cancel` | `compute_job_cancel` | 20 |
| dataset | PUT `/datasets/:id` | `dataset_update` | 20 |
| dataset | PUT `/datasets/:id/datasheet` | `datasheet_update` | 30 |
| dataset | POST `/datasets/:id/upload/complete` | `upload_complete` | 10 |
| delivery | POST `/orders/:id/download` | `download_request` | 30 |
| notification | POST `/users/me/notifications/read-all` | `notif_read_all` | 20 |
| notification | POST `/users/me/notifications/:id/read` | `notif_read` | 30 |
| order | POST `/orders/:id/confirm-delivery` | `order_confirm` | 20 |
| order | POST `/orders/:id/review` | `order_review` | 30 |

`anomaly`, `compliance`, and `notification` did not previously take a limiter;
their `Register` signatures gained a `ratelimit.Limiter` param, wired from the
shared `lim := s.limiter()` in `server.go` (the same limiter every other module
uses — Redis-backed with in-memory fallback).

The limiter mechanism itself is unit-tested in
`internal/platform/middleware/ratelimit_test.go`; per-route application is
declarative config, consistent with how every other rate-limited route in the
codebase is defined.
