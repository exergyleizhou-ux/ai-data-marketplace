-- Revert to the original type whitelist. Fails if any pii_redaction/authenticity
-- rows exist (expected — tightening a CHECK requires the data to already comply).
ALTER TABLE quality_checks DROP CONSTRAINT quality_checks_type_check;
ALTER TABLE quality_checks ADD CONSTRAINT quality_checks_type_check
    CHECK (type IN ('format', 'stats', 'dedup', 'pii'));
