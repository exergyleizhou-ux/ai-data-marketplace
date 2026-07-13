CREATE TABLE workbench_quota_policies (
  workspace_id uuid PRIMARY KEY,
  account_id uuid NOT NULL,
  user_concurrent_runs integer NOT NULL DEFAULT 3 CHECK (user_concurrent_runs > 0),
  workspace_concurrent_runs integer NOT NULL DEFAULT 10 CHECK (workspace_concurrent_runs > 0),
  monthly_tokens bigint NOT NULL DEFAULT 10000000 CHECK (monthly_tokens >= 0),
  monthly_compute_millis bigint NOT NULL DEFAULT 36000000 CHECK (monthly_compute_millis >= 0),
  storage_bytes bigint NOT NULL DEFAULT 1073741824 CHECK (storage_bytes >= 0),
  artifact_total_bytes bigint NOT NULL DEFAULT 268435456 CHECK (artifact_total_bytes >= 0),
  artifact_single_bytes bigint NOT NULL DEFAULT 8388608 CHECK (artifact_single_bytes >= 0),
  run_wall_millis bigint NOT NULL DEFAULT 3600000 CHECK (run_wall_millis > 0),
  run_max_steps bigint NOT NULL DEFAULT 200 CHECK (run_max_steps > 0),
  run_max_events bigint NOT NULL DEFAULT 10000 CHECK (run_max_events > 0),
  event_max_bytes bigint NOT NULL DEFAULT 262144 CHECK (event_max_bytes > 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT workbench_quota_policy_owner_fk FOREIGN KEY (workspace_id, account_id)
    REFERENCES workbench_workspaces(id, account_id) ON DELETE CASCADE
);

ALTER TABLE workbench_usage ADD COLUMN compute_millis bigint NOT NULL DEFAULT 0 CHECK (compute_millis >= 0);

CREATE TABLE workbench_monthly_ledgers (
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  month_start date NOT NULL,
  tokens bigint NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  compute_millis bigint NOT NULL DEFAULT 0 CHECK (compute_millis >= 0),
  cost_microunits bigint NOT NULL DEFAULT 0 CHECK (cost_microunits >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, workspace_id, month_start),
  CONSTRAINT workbench_monthly_owner_fk FOREIGN KEY (workspace_id, account_id)
    REFERENCES workbench_workspaces(id, account_id) ON DELETE CASCADE
);

CREATE TABLE workbench_run_reservations (
  -- Admission happens before Lumen creates the durable run row, so this key
  -- intentionally cannot reference workbench_runs.
  run_id text PRIMARY KEY,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  state text NOT NULL DEFAULT 'active' CHECK (state IN ('active','released')),
  event_count bigint NOT NULL DEFAULT 0 CHECK (event_count >= 0),
  step_count bigint NOT NULL DEFAULT 0 CHECK (step_count >= 0),
  artifact_bytes bigint NOT NULL DEFAULT 0 CHECK (artifact_bytes >= 0),
  started_at timestamptz NOT NULL,
  released_at timestamptz,
  CONSTRAINT workbench_run_reservation_owner_fk FOREIGN KEY(workspace_id,account_id)
    REFERENCES workbench_workspaces(id,account_id) ON DELETE CASCADE
);
CREATE INDEX workbench_run_reservations_active_owner_idx
  ON workbench_run_reservations(workspace_id, account_id) WHERE state='active';

INSERT INTO workbench_run_reservations(run_id,account_id,workspace_id,state,started_at)
SELECT id,account_id,workspace_id,'active',COALESCE(started_at,created_at)
FROM workbench_runs
WHERE status NOT IN ('completed','failed','canceled','cancelled')
ON CONFLICT(run_id) DO NOTHING;

CREATE TABLE workbench_usage_debits (
  run_id text NOT NULL,
  event_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  month_start date NOT NULL,
  tokens bigint NOT NULL CHECK(tokens >= 0),
  compute_millis bigint NOT NULL CHECK(compute_millis >= 0),
  cost_microunits bigint NOT NULL CHECK(cost_microunits >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id,event_id),
  CONSTRAINT workbench_usage_debit_owner_fk FOREIGN KEY(workspace_id,account_id)
    REFERENCES workbench_workspaces(id,account_id) ON DELETE CASCADE
);

CREATE TABLE workbench_artifact_reservations (
  artifact_id text PRIMARY KEY,
  run_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  state text NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','committed','released')),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT workbench_artifact_reservation_run_owner_fk FOREIGN KEY (run_id, account_id, workspace_id)
    REFERENCES workbench_runs(id, account_id, workspace_id) ON DELETE CASCADE
);
CREATE INDEX workbench_artifact_reservations_owner_idx
  ON workbench_artifact_reservations(workspace_id,account_id,run_id) WHERE state!='released';

INSERT INTO workbench_artifact_reservations(artifact_id,run_id,account_id,workspace_id,size_bytes,state,created_at)
SELECT id,run_id,account_id,workspace_id,size_bytes,'committed',created_at
FROM workbench_artifacts
ON CONFLICT(artifact_id) DO NOTHING;
