CREATE TABLE workbench_runs (
  id text PRIMARY KEY,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  profile text NOT NULL CHECK (profile IN ('code', 'lab')),
  status text NOT NULL,
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  title text NOT NULL DEFAULT '',
  request jsonb NOT NULL DEFAULT '{}'::jsonb,
  result jsonb,
  error_code text,
  error_message text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT workbench_runs_owner_fk FOREIGN KEY (workspace_id, account_id)
    REFERENCES workbench_workspaces(id, account_id) ON DELETE CASCADE,
  UNIQUE (id, account_id, workspace_id)
);
CREATE INDEX workbench_runs_owner_updated_idx
  ON workbench_runs(account_id, workspace_id, updated_at DESC);

CREATE TABLE workbench_events (
  id text NOT NULL,
  run_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  seq bigint NOT NULL CHECK (seq > 0),
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, seq),
  UNIQUE (run_id, id),
  CONSTRAINT workbench_events_run_owner_fk FOREIGN KEY (run_id, account_id, workspace_id)
    REFERENCES workbench_runs(id, account_id, workspace_id) ON DELETE CASCADE
);
CREATE INDEX workbench_events_owner_run_idx
  ON workbench_events(account_id, workspace_id, run_id, seq);

CREATE TABLE workbench_approvals (
  approval_id text PRIMARY KEY,
  run_id text NOT NULL,
  tool_call_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  owner text NOT NULL,
  risk_level text NOT NULL,
  reason text NOT NULL,
  effects jsonb NOT NULL DEFAULT '{}'::jsonb,
  command text,
  file_scope jsonb NOT NULL DEFAULT '[]'::jsonb,
  remote_target text,
  network_targets jsonb NOT NULL DEFAULT '[]'::jsonb,
  estimated_cost bigint NOT NULL DEFAULT 0 CHECK (estimated_cost >= 0),
  expected_outputs jsonb NOT NULL DEFAULT '[]'::jsonb,
  args_hash text NOT NULL CHECK (args_hash ~ '^[0-9a-f]{64}$'),
  editable_args jsonb NOT NULL DEFAULT '{}'::jsonb,
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  decided_at timestamptz,
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decision text CHECK (decision IS NULL OR decision IN ('approved', 'rejected', 'expired', 'invalidated')),
  CONSTRAINT workbench_approvals_run_owner_fk FOREIGN KEY (run_id, account_id, workspace_id)
    REFERENCES workbench_runs(id, account_id, workspace_id) ON DELETE CASCADE,
  CONSTRAINT workbench_approvals_decision_complete CHECK (
    (decision IS NULL AND decided_at IS NULL AND decided_by IS NULL) OR
    (decision IS NOT NULL AND decided_at IS NOT NULL)
  ),
  UNIQUE (run_id, tool_call_id, args_hash)
);
CREATE INDEX workbench_approvals_owner_pending_idx
  ON workbench_approvals(account_id, workspace_id, created_at DESC) WHERE decision IS NULL;

CREATE TABLE workbench_artifacts (
  id text PRIMARY KEY,
  run_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  name text NOT NULL,
  kind text NOT NULL,
  media_type text NOT NULL DEFAULT 'application/octet-stream',
  object_key text NOT NULL,
  sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT workbench_artifacts_run_owner_fk FOREIGN KEY (run_id, account_id, workspace_id)
    REFERENCES workbench_runs(id, account_id, workspace_id) ON DELETE CASCADE,
  UNIQUE (workspace_id, object_key)
);
CREATE INDEX workbench_artifacts_owner_run_idx
  ON workbench_artifacts(account_id, workspace_id, run_id, created_at);

CREATE TABLE workbench_usage (
  id bigserial PRIMARY KEY,
  run_id text NOT NULL,
  event_id text NOT NULL,
  account_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  provider text NOT NULL,
  model text NOT NULL,
  input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  cache_read_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
  cache_write_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
  cost_microunits bigint NOT NULL DEFAULT 0 CHECK (cost_microunits >= 0),
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT workbench_usage_run_owner_fk FOREIGN KEY (run_id, account_id, workspace_id)
    REFERENCES workbench_runs(id, account_id, workspace_id) ON DELETE CASCADE,
  UNIQUE (run_id, event_id)
);
CREATE INDEX workbench_usage_owner_occurred_idx
  ON workbench_usage(account_id, workspace_id, occurred_at DESC);
