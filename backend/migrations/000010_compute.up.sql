-- Compute-to-Data (隐私计算 / 可用不可见) — Phase 1 schema (design doc §4 + §15).
-- Adds a "sandbox compute" product alongside the existing download product:
-- a buyer purchases a compute entitlement, submits a WHITELISTED algorithm job,
-- and receives the OUTPUT (model / metrics) — never the raw data.
--
-- Conventions match 000001..000009: UUID PKs via gen_random_uuid() (PG13+, no
-- extension); money as BIGINT cents; time as TIMESTAMPTZ.
--
-- Hardening columns carried from the v1.1 design (do NOT defer these — adding
-- them later is costly and invites concurrency bugs):
--   * algorithms.image_digest  — pin the image by sha256 (no mutable :latest)
--   * compute_entitlements.status — refund/dispute revocation (ties to H2)
--   * compute_jobs.idempotency_key / attempts / lease_until — idempotent submit
--     + crash recovery (lease + retry), mirroring the H3 settlement model.

-- Registered algorithms: platform-audited trusted whitelist (+ later, custom images).
CREATE TABLE IF NOT EXISTS algorithms (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      UUID REFERENCES users (id),           -- NULL = platform built-in
    name          TEXT NOT NULL,
    runtime       TEXT NOT NULL,                         -- python-sklearn | python-lightgbm | sql | custom-image
    image         TEXT NOT NULL,                         -- container image reference
    image_digest  TEXT NOT NULL DEFAULT '',              -- sha256:... content digest; trusted algos MUST pin (no :latest)
    version       INT  NOT NULL DEFAULT 1,               -- bump on any code/image change; review targets a version
    source_ref    TEXT NOT NULL DEFAULT '',              -- source / audit-ticket ref; required for trusted
    entrypoint    TEXT NOT NULL DEFAULT '',
    params_schema JSONB,                                  -- JSON Schema for params (frontend form + backend validate)
    output_kind   TEXT NOT NULL                          -- model | metrics | table | aggregate
                       CHECK (output_kind IN ('model','metrics','table','aggregate')),
    status        TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','approved','rejected','disabled')),
    trusted       BOOLEAN NOT NULL DEFAULT false,         -- audited; may run on sensitive data / produce models
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_algorithms_status ON algorithms (status);

-- Per-dataset sandbox sale config (coexists with the download product; a dataset
-- may be sold both ways).
CREATE TABLE IF NOT EXISTS dataset_compute_offers (
    dataset_id            UUID PRIMARY KEY REFERENCES datasets (id) ON DELETE CASCADE,
    enabled               BOOLEAN NOT NULL DEFAULT false,
    allow_custom          BOOLEAN NOT NULL DEFAULT false, -- allow buyer-supplied algorithms (false = whitelist only)
    allowed_algorithm_ids UUID[] NOT NULL DEFAULT '{}',   -- seller-permitted subset (empty = all approved)
    price_cents           BIGINT NOT NULL DEFAULT 0,
    max_runtime_secs      INT NOT NULL DEFAULT 1800,
    max_output_bytes      BIGINT NOT NULL DEFAULT 10485760, -- 10 MiB output cap (anti whole-dataset dump)
    max_output_files      INT NOT NULL DEFAULT 16,
    dp_epsilon            DOUBLE PRECISION,                 -- default per-job DP budget (aggregate outputs)
    dp_epsilon_total      DOUBLE PRECISION,                 -- per-buyer cumulative ε ceiling on this dataset
    return_logs           BOOLEAN NOT NULL DEFAULT false,   -- return algo logs to buyer? default no (exfil side channel)
    review_output         BOOLEAN NOT NULL DEFAULT false,   -- require ops human review before release
    trust_level           TEXT NOT NULL DEFAULT 'L1'
                              CHECK (trust_level IN ('L1','L2','L3')),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Buyer compute entitlement (one purchase may carry N job credits).
CREATE TABLE IF NOT EXISTS compute_entitlements (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id    UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    buyer_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    order_id      UUID REFERENCES orders (id),            -- reuse existing payment/settlement
    jobs_quota    INT NOT NULL DEFAULT 1,
    jobs_used     INT NOT NULL DEFAULT 0,                 -- decremented ATOMICALLY (see repo.SpendQuota)
    status        TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active','exhausted','expired','revoked')),
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_compute_entitlements_buyer ON compute_entitlements (buyer_id, dataset_id);
CREATE INDEX IF NOT EXISTS idx_compute_entitlements_order ON compute_entitlements (order_id);

-- Compute jobs (the C2D unit of work).
CREATE TABLE IF NOT EXISTS compute_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id        UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    version_id        UUID REFERENCES dataset_versions (id),
    buyer_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    entitlement_id    UUID NOT NULL REFERENCES compute_entitlements (id),
    algorithm_id      UUID REFERENCES algorithms (id),
    algorithm_version INT,                                 -- pinned at submit (reproducible / disputable re-run)
    params            JSONB,
    idempotency_key   TEXT,                                 -- dedupe repeat submits under one entitlement
    status            TEXT NOT NULL DEFAULT 'created'
                          CHECK (status IN ('created','queued','running','output_pending',
                                            'output_reviewing','released','failed','rejected','canceled')),
    attempts          INT NOT NULL DEFAULT 0,               -- crash-retry counter; >= max -> failed
    runner_id         TEXT,                                 -- runner instance holding the lease
    lease_until       TIMESTAMPTZ,                          -- lease expiry; expired+running => crashed => reclaim
    dp_epsilon        DOUBLE PRECISION,
    output_key        TEXT,                                 -- output object-storage key
    output_bytes      BIGINT,
    output_kind       TEXT,
    logs_key          TEXT,                                 -- vetted algorithm logs (only if offer.return_logs)
    error             TEXT,                                 -- de-identified error code/summary (never raw stdout)
    attestation       JSONB,                                -- L2 TEE remote-attestation report (Phase 3)
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_compute_jobs_buyer  ON compute_jobs (buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_compute_jobs_status ON compute_jobs (status);
-- Idempotency: one job per (entitlement, idempotency_key).
CREATE UNIQUE INDEX IF NOT EXISTS idx_compute_jobs_idem ON compute_jobs (entitlement_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
-- Stale-lease reclaim scan.
CREATE INDEX IF NOT EXISTS idx_compute_jobs_lease ON compute_jobs (status, lease_until) WHERE status = 'running';

-- Per (dataset, buyer) DP budget ledger (cumulative ε; ceiling lives in the offer).
CREATE TABLE IF NOT EXISTS dp_budget_ledger (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id    UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    buyer_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id        UUID REFERENCES compute_jobs (id),
    epsilon_spent DOUBLE PRECISION NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dp_budget_ledger_scope ON dp_budget_ledger (dataset_id, buyer_id);
