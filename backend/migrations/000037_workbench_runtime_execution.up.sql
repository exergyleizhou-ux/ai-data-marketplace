ALTER TABLE workbench_approvals
  ADD COLUMN step_id text,
  ADD COLUMN executed_at timestamptz,
  ADD COLUMN execution_id text,
  ADD COLUMN execution_state text NOT NULL DEFAULT 'pending';

ALTER TABLE workbench_approvals
  ADD CONSTRAINT workbench_approvals_execution_state_check
    CHECK (execution_state IN ('pending', 'consumed', 'executed', 'failed')),
  ADD CONSTRAINT workbench_approvals_execution_lifecycle_check CHECK (
    (execution_state = 'pending' AND execution_id IS NULL AND executed_at IS NULL) OR
    (execution_state = 'consumed' AND execution_id IS NOT NULL AND executed_at IS NULL AND decision = 'approved') OR
    (execution_state IN ('executed', 'failed') AND execution_id IS NOT NULL AND executed_at IS NOT NULL AND decision = 'approved')
  );
CREATE UNIQUE INDEX workbench_approvals_execution_id_unique_idx
  ON workbench_approvals(execution_id) WHERE execution_id IS NOT NULL;

ALTER TABLE workbench_artifacts
  ADD COLUMN step_id text,
  ADD COLUMN tool_call_id text,
  ADD COLUMN model text,
  ADD COLUMN input_refs jsonb NOT NULL DEFAULT '[]'::jsonb;
CREATE UNIQUE INDEX workbench_artifacts_run_tool_call_unique_idx
  ON workbench_artifacts(run_id, tool_call_id) WHERE tool_call_id IS NOT NULL;
