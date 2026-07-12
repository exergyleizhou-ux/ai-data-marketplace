ALTER TABLE workbench_run_reservations
  ADD COLUMN last_heartbeat_at timestamptz,
  ADD COLUMN expires_at timestamptz;

UPDATE workbench_run_reservations
SET last_heartbeat_at = COALESCE(released_at, started_at),
    expires_at = CASE
      WHEN state = 'released' THEN COALESCE(released_at, started_at)
      ELSE now() + interval '2 minutes'
    END;

ALTER TABLE workbench_run_reservations
  ALTER COLUMN last_heartbeat_at SET NOT NULL,
  ALTER COLUMN expires_at SET NOT NULL;

CREATE INDEX workbench_run_reservations_expiry_idx
  ON workbench_run_reservations(expires_at) WHERE state='active';

ALTER TABLE workbench_artifact_reservations
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
