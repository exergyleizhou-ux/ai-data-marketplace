BEGIN;

-- Reviews gain a moderation flag (questions already use status='hidden').
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT false;

-- User-submitted reports against a question or review. Cross-table by design
-- (target_type + target_id), so no FK on the target — integrity is enforced at
-- the service layer. reporter_id keeps a soft attribution for abuse tracking.
CREATE TABLE IF NOT EXISTS content_reports (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id  UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type  TEXT NOT NULL CHECK (target_type IN ('question', 'review')),
    target_id    UUID NOT NULL,
    reason       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolution   TEXT CHECK (resolution IN ('hide', 'dismiss')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at  TIMESTAMPTZ,
    resolved_by  UUID REFERENCES users (id)
);

-- One open report per (reporter, target): re-reporting the same item is a no-op
-- while a report is still open, preventing report-spam from inflating the queue.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_content_reports_open
    ON content_reports (reporter_id, target_type, target_id)
    WHERE status = 'open';

CREATE INDEX IF NOT EXISTS idx_content_reports_status ON content_reports (status, created_at DESC);

COMMIT;
