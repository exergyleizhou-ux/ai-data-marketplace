-- 000014_notifications_certificates
-- Adds user-facing notifications for key platform events
-- and a lightweight certificate lookup table for public verification.

CREATE TABLE IF NOT EXISTS notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT NOT NULL,
    kind        TEXT NOT NULL,  -- order_paid|order_settled|order_disputed|quality_done|compute_released
    title       TEXT NOT NULL,
    body        TEXT,
    resource_type TEXT,         -- order|dataset|compute_job
    resource_id TEXT,
    is_read     BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications (user_id, created_at DESC) WHERE is_read = false;

CREATE TABLE IF NOT EXISTS certificates (
    cert_id       TEXT PRIMARY KEY,  -- VO-<12 hex>
    resource_type TEXT NOT NULL,     -- dataset|compute_job|federated_job
    resource_id   TEXT NOT NULL,     -- the UUID of the source record
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_certificates_resource
    ON certificates (resource_type, resource_id);
