-- P4-a Federated Learning MVP (design doc P4 §2.1/§3 + spec 2026-06-03).
-- A federated job references N datasets, fans out N sandbox sub-jobs (existing
-- compute_jobs tagged with federated_job_id), and aggregates their local model
-- params with FedAvg into a joint model. Raw data never leaves each sandbox.
--
-- Conventions match 000010/000011: UUID PKs via gen_random_uuid(), TIMESTAMPTZ.
-- New columns are nullable or defaulted (safe online add); the partial index is
-- scoped to federated sub-jobs only.

CREATE TABLE IF NOT EXISTS compute_federated_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id         UUID NOT NULL REFERENCES users (id),
    algorithm_id     UUID REFERENCES algorithms (id),       -- e.g. fed-logreg
    dataset_ids      UUID[] NOT NULL,                       -- participating parties
    mode             TEXT NOT NULL DEFAULT 'federated',     -- 'federated' | 'mpc' (reserved)
    status           TEXT NOT NULL DEFAULT 'created'        -- created→fanout→aggregating→released/failed/rejected
                          CHECK (status IN ('created','fanout','aggregating','released','failed','rejected')),
    min_participants INT  NOT NULL DEFAULT 0,               -- 0 ⇒ all datasets (MVP); fault tolerance later
    params           JSONB NOT NULL DEFAULT '{}',
    dp_epsilon       DOUBLE PRECISION,
    output_key       TEXT,                                  -- joint model object key (released only)
    output_bytes     BIGINT NOT NULL DEFAULT 0,
    failure_code     TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sub-jobs reuse compute_jobs; this nullable FK links them to their federated parent.
ALTER TABLE compute_jobs ADD COLUMN IF NOT EXISTS federated_job_id UUID REFERENCES compute_federated_jobs (id);
CREATE INDEX IF NOT EXISTS idx_compute_jobs_federated ON compute_jobs (federated_job_id) WHERE federated_job_id IS NOT NULL;

-- Sellers opt a dataset into federated use.
ALTER TABLE dataset_compute_offers ADD COLUMN IF NOT EXISTS allow_federated BOOLEAN NOT NULL DEFAULT false;
