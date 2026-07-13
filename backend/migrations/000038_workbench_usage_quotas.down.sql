DROP TABLE IF EXISTS workbench_artifact_reservations;
DROP TABLE IF EXISTS workbench_usage_debits;
DROP TABLE IF EXISTS workbench_run_reservations;
DROP TABLE IF EXISTS workbench_monthly_ledgers;
DROP TABLE IF EXISTS workbench_quota_policies;
ALTER TABLE workbench_usage DROP COLUMN IF EXISTS compute_millis;
