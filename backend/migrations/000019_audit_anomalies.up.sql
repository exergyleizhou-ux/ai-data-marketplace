-- 000019: audit anomaly detections (computed periodically; ops triage).
CREATE TABLE IF NOT EXISTS audit_anomalies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL,
    actor_id        UUID,
    resource_pattern TEXT NOT NULL,
    sample_audit_ids BIGINT[] NOT NULL,
    count           INT NOT NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'acknowledged', 'resolved')),
    ops_note        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dedup: only one open anomaly per kind/actor/pattern.
-- DATE() / ::date is STABLE not IMMUTABLE so cannot appear in a real unique index.
-- We enforce dedup in the Upsert method via ON CONFLICT DO NOTHING on this partial
-- unique index, which uses only IMMUTABLE expressions.
CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_anomalies_dedup
    ON audit_anomalies (kind, COALESCE(actor_id::text,''), resource_pattern)
    WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_audit_anomalies_open
    ON audit_anomalies (status, last_seen_at DESC) WHERE status = 'open';
