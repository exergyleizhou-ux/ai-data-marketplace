BEGIN;

DROP TRIGGER IF EXISTS trg_audit_logs_no_update ON audit_logs;
DROP FUNCTION IF EXISTS audit_logs_block_mutation();

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS disputes;
DROP TABLE IF EXISTS settlements;
DROP TABLE IF EXISTS deliveries;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS quality_checks;
DROP TABLE IF EXISTS dataset_files;
-- Break the datasets <-> dataset_versions cycle before dropping.
ALTER TABLE datasets DROP CONSTRAINT IF EXISTS fk_datasets_current_version;
DROP TABLE IF EXISTS dataset_versions;
DROP TABLE IF EXISTS datasets;
DROP TABLE IF EXISTS payout_accounts;
DROP TABLE IF EXISTS kyc_records;
DROP TABLE IF EXISTS users;

COMMIT;
