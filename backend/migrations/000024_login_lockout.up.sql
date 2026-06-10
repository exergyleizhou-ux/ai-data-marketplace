BEGIN;
-- Per-account login lockout (anti-bruteforce). IP rate limiting already
-- exists; this closes the rotating-IP attack on a single account. Only
-- existing accounts are tracked (no row per probe of unknown accounts).
CREATE TABLE IF NOT EXISTS login_lockouts (
    user_id      UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    failures     INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMIT;
