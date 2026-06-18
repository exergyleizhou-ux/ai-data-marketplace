-- quality_checks had no unique key on (version_id, type), and the quality retry
-- queue is at-least-once, so a re-run of processQuality appended a second full
-- set of check rows — duplicating the buyer-facing report and skewing the
-- quality ranking aggregates. Make (version_id, type) unique so SaveQualityCheck
-- can upsert idempotently.

-- Collapse any pre-existing duplicates, keeping the most recent row per key.
DELETE FROM quality_checks q
USING (
    SELECT id,
           row_number() OVER (PARTITION BY version_id, type ORDER BY created_at DESC, id DESC) AS rn
    FROM quality_checks
) d
WHERE q.id = d.id AND d.rn > 1;

ALTER TABLE quality_checks
    ADD CONSTRAINT quality_checks_version_type_key UNIQUE (version_id, type);
