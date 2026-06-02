ALTER TABLE quality_checks DROP CONSTRAINT quality_checks_type_check;
ALTER TABLE quality_checks ADD CONSTRAINT quality_checks_type_check
    CHECK (type IN ('format', 'stats', 'dedup', 'pii', 'pii_redaction', 'authenticity'));
