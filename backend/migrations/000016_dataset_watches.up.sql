-- 000016: buyer watchlist + last-notified version tracking.
CREATE TABLE IF NOT EXISTS dataset_watches (
    user_id                    TEXT NOT NULL,
    dataset_id                 UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    last_notified_version_id   UUID,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, dataset_id)
);

CREATE INDEX IF NOT EXISTS idx_dataset_watches_dataset ON dataset_watches (dataset_id);
CREATE INDEX IF NOT EXISTS idx_dataset_watches_user    ON dataset_watches (user_id, created_at DESC);
