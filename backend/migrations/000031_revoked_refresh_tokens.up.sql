-- Durable, cross-instance refresh-token revocation.
--
-- Previously the revocation denylist (logout + rotation reuse-detection) lived
-- only in Redis, falling back to a per-process in-memory map when Redis was
-- unavailable. That fallback is fail-open: a revoked/logged-out token stays
-- valid on other instances and is resurrected on restart (refresh TTL is 30
-- days). Postgres is the shared source of truth and is already required by the
-- refresh path, so we store revoked jtis here. IsRevoked ignores expired rows;
-- a background cleaner purges them to bound the table.
CREATE TABLE revoked_refresh_tokens (
    jti        text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);

-- Supports the cleaner's range delete on expiry.
CREATE INDEX idx_revoked_refresh_tokens_expires ON revoked_refresh_tokens (expires_at);
