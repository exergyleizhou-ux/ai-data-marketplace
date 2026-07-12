DROP INDEX IF EXISTS workbench_artifacts_run_tool_call_unique_idx;
ALTER TABLE workbench_artifacts
  DROP COLUMN IF EXISTS input_refs,
  DROP COLUMN IF EXISTS model,
  DROP COLUMN IF EXISTS tool_call_id,
  DROP COLUMN IF EXISTS step_id;

DROP INDEX IF EXISTS workbench_approvals_execution_id_unique_idx;
ALTER TABLE workbench_approvals
  DROP CONSTRAINT IF EXISTS workbench_approvals_execution_lifecycle_check,
  DROP CONSTRAINT IF EXISTS workbench_approvals_execution_state_check,
  DROP COLUMN IF EXISTS execution_state,
  DROP COLUMN IF EXISTS execution_id,
  DROP COLUMN IF EXISTS executed_at,
  DROP COLUMN IF EXISTS step_id;
