-- 000015: persist quality jobs so transient failures retry and process restarts don't lose work.
CREATE TABLE IF NOT EXISTS quality_retries (
    dataset_id     UUID PRIMARY KEY REFERENCES datasets(id) ON DELETE CASCADE,
    version_id     UUID NOT NULL,
    content_sha256 TEXT NOT NULL,
    attempts       INT  NOT NULL DEFAULT 0,
    max_attempts   INT  NOT NULL DEFAULT 3,
    next_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_quality_retries_due
    ON quality_retries (next_at) WHERE attempts < max_attempts;
