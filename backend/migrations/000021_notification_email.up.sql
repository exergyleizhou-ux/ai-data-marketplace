-- 000021: notification preferences + email send log.
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    email_enabled  BOOLEAN NOT NULL DEFAULT true,
    in_app_enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind)
);

CREATE TABLE IF NOT EXISTS email_send_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    kind            TEXT NOT NULL,
    to_address      TEXT NOT NULL,
    subject         TEXT NOT NULL,
    status          TEXT NOT NULL,
    error           TEXT,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    idempotency_key TEXT UNIQUE
);

CREATE INDEX IF NOT EXISTS idx_email_send_log_user
    ON email_send_log (user_id, sent_at DESC);
