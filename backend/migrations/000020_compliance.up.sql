-- 000020: PIPL Article 45 (data export) + Article 47 (account deletion).

CREATE TABLE IF NOT EXISTS data_export_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'generating', 'ready', 'failed', 'expired')),
    object_key      TEXT,
    object_bytes    BIGINT,
    expires_at      TIMESTAMPTZ,
    error           TEXT,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_data_export_user_recent
    ON data_export_jobs (user_id, requested_at DESC);

CREATE TABLE IF NOT EXISTS account_deletion_requests (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id),
    reason            TEXT,
    status            TEXT NOT NULL DEFAULT 'cooling'
                          CHECK (status IN ('cooling', 'approved', 'rejected', 'cancelled', 'deleted')),
    cooling_until     TIMESTAMPTZ NOT NULL,
    ops_note          TEXT,
    requested_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at      TIMESTAMPTZ,
    processed_by      UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_account_deletion_pending
    ON account_deletion_requests (status, cooling_until)
    WHERE status IN ('cooling', 'approved');

CREATE UNIQUE INDEX IF NOT EXISTS uq_account_deletion_active_per_user
    ON account_deletion_requests (user_id)
    WHERE status IN ('cooling', 'approved');
