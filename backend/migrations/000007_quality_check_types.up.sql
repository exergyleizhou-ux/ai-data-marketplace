-- Extend the quality_checks.type whitelist for the de-identification
-- verification row (pii_redaction) and the statistical data-authenticity
-- screening row (authenticity). The original inline CHECK from 000001 is named
-- quality_checks_type_check by Postgres convention.
ALTER TABLE quality_checks DROP CONSTRAINT quality_checks_type_check;
ALTER TABLE quality_checks ADD CONSTRAINT quality_checks_type_check
    CHECK (type IN ('format', 'stats', 'dedup', 'pii', 'pii_redaction', 'authenticity'));
