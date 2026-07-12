ALTER TABLE workbench_artifact_reservations DROP COLUMN IF EXISTS updated_at;
DROP INDEX IF EXISTS workbench_run_reservations_expiry_idx;
ALTER TABLE workbench_run_reservations
  DROP COLUMN IF EXISTS expires_at,
  DROP COLUMN IF EXISTS last_heartbeat_at;
