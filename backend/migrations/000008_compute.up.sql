-- 000008_compute.up.sql
-- C2D (Compute-to-Data): algorithms, jobs, and attestation chain.

CREATE TABLE IF NOT EXISTS compute_algorithms (
    id             TEXT PRIMARY KEY,
    seller_id      TEXT NOT NULL REFERENCES users(id),
    name           TEXT NOT NULL,
    runtime        TEXT NOT NULL DEFAULT 'docker',
    image          TEXT NOT NULL,
    image_digest   TEXT NOT NULL DEFAULT '',
    version        INT NOT NULL DEFAULT 1,
    source_ref     TEXT NOT NULL DEFAULT '',
    entrypoint     TEXT NOT NULL DEFAULT '',
    output_kind    TEXT NOT NULL DEFAULT 'model',
    params_schema  TEXT NOT NULL DEFAULT '{}',
    current_version BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compute_algo_seller ON compute_algorithms(seller_id);
CREATE INDEX IF NOT EXISTS idx_compute_algo_current ON compute_algorithms(current_version) WHERE current_version = true;
CREATE UNIQUE INDEX IF NOT EXISTS idx_compute_algo_unique ON compute_algorithms(name, version);

CREATE TABLE IF NOT EXISTS compute_jobs (
    id                TEXT PRIMARY KEY,
    algorithm_id      TEXT NOT NULL REFERENCES compute_algorithms(id),
    buyer_id          TEXT NOT NULL REFERENCES users(id),
    dataset_id        TEXT NOT NULL REFERENCES datasets(id),
    params            TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'pending',
    output_kind       TEXT NOT NULL DEFAULT '',
    output_bytes      BIGINT NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    attest_input_hash TEXT NOT NULL DEFAULT '',
    attest_output_hash TEXT NOT NULL DEFAULT '',
    attest_signature  TEXT NOT NULL DEFAULT '',
    attest_signed_at  TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compute_job_buyer ON compute_jobs(buyer_id);
CREATE INDEX IF NOT EXISTS idx_compute_job_status ON compute_jobs(status);
CREATE INDEX IF NOT EXISTS idx_compute_job_algo ON compute_jobs(algorithm_id);
