CREATE TABLE workbench_workspaces (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug text NOT NULL,
  display_name text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'active',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, slug),
  UNIQUE (id, account_id)
);
CREATE INDEX workbench_workspaces_account_idx ON workbench_workspaces(account_id);
