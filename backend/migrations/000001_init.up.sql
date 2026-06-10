-- P3 initial schema. See docs §5.3.
-- Conventions: UUID PKs via gen_random_uuid() (Postgres 13+, no extension);
-- money as BIGINT cents (never float); timestamps TIMESTAMPTZ in UTC.

BEGIN;

-- ---------------------------------------------------------------------------
-- users / identity
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account       TEXT NOT NULL UNIQUE,
    account_type  TEXT NOT NULL CHECK (account_type IN ('phone', 'email')),
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'buyer'
                       CHECK (role IN ('buyer', 'seller', 'both', 'ops', 'admin')),
    kyc_status    TEXT NOT NULL DEFAULT 'none'
                       CHECK (kyc_status IN ('none', 'pending', 'verified', 'rejected')),
    status        TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'frozen')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kyc_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type            TEXT NOT NULL CHECK (type IN ('personal', 'company')),
    real_name       TEXT,
    company_name    TEXT,
    id_no_hash      TEXT,                 -- desensitized; never store raw ID number
    material_urls   JSONB NOT NULL DEFAULT '[]'::jsonb,
    verify_status   TEXT NOT NULL DEFAULT 'pending'
                         CHECK (verify_status IN ('pending', 'verified', 'rejected')),
    verify_provider TEXT,
    reviewed_by     UUID REFERENCES users (id),
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_kyc_records_user ON kyc_records (user_id);

-- Split-settlement receiving account (compliance-critical, see docs §2.1).
CREATE TABLE IF NOT EXISTS payout_accounts (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel                  TEXT NOT NULL CHECK (channel IN ('wechat', 'alipay')),
    account_ref              TEXT NOT NULL,  -- channel-side split receiver id
    name_consistency_checked BOOLEAN NOT NULL DEFAULT false,
    authorized_at            TIMESTAMPTZ,
    status                   TEXT NOT NULL DEFAULT 'active'
                                  CHECK (status IN ('active', 'disabled')),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payout_accounts_user ON payout_accounts (user_id);

-- ---------------------------------------------------------------------------
-- datasets (datasets <-> dataset_versions is circular: current_version_id FK
-- is added after dataset_versions exists)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS datasets (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id             UUID NOT NULL REFERENCES users (id),
    title                 TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    data_type             TEXT NOT NULL CHECK (data_type IN ('text', 'code', 'structured')),
    domain                TEXT,
    license_type          TEXT NOT NULL
                               CHECK (license_type IN ('commercial', 'research', 'train_only')),
    suggested_price_cents BIGINT,
    final_price_cents     BIGINT,
    status                TEXT NOT NULL DEFAULT 'draft'
                               CHECK (status IN ('draft', 'uploading', 'checking',
                                                 'reviewing', 'published', 'rejected', 'delisted')),
    total_size_bytes      BIGINT NOT NULL DEFAULT 0,
    sample_count          BIGINT NOT NULL DEFAULT 0,
    source_declaration    JSONB,
    source_signed_at      TIMESTAMPTZ,
    current_version_id    UUID,           -- FK added below
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_datasets_seller ON datasets (seller_id);
CREATE INDEX IF NOT EXISTS idx_datasets_status ON datasets (status);

CREATE TABLE IF NOT EXISTS dataset_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id     UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    version_no     INTEGER NOT NULL,
    manifest       JSONB,                 -- file listing
    content_sha256 TEXT,                  -- whole-content fingerprint
    simhash        TEXT,                  -- near-duplicate fingerprint (64-bit, hex)
    changelog      TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (dataset_id, version_no)
);

ALTER TABLE datasets
    ADD CONSTRAINT fk_datasets_current_version
    FOREIGN KEY (current_version_id) REFERENCES dataset_versions (id);

CREATE TABLE IF NOT EXISTS dataset_files (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id   UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    version_id   UUID NOT NULL REFERENCES dataset_versions (id) ON DELETE CASCADE,
    object_key   TEXT NOT NULL,           -- key in OSS/COS; bytes never in PG
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    sha256       TEXT,
    content_type TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dataset_files_version ON dataset_files (version_id);

CREATE TABLE IF NOT EXISTS quality_checks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id UUID NOT NULL REFERENCES datasets (id) ON DELETE CASCADE,
    version_id UUID NOT NULL REFERENCES dataset_versions (id) ON DELETE CASCADE,
    type       TEXT NOT NULL CHECK (type IN ('format', 'stats', 'dedup', 'pii')),
    result     TEXT NOT NULL CHECK (result IN ('pass', 'warn', 'fail')),
    report     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_quality_checks_version ON quality_checks (version_id);

-- ---------------------------------------------------------------------------
-- orders & money (state machine: docs §5.4)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS orders (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id           UUID NOT NULL REFERENCES users (id),
    seller_id          UUID NOT NULL REFERENCES users (id),
    dataset_id         UUID NOT NULL REFERENCES datasets (id),
    version_id         UUID NOT NULL REFERENCES dataset_versions (id),
    license_type       TEXT NOT NULL,
    amount_cents       BIGINT NOT NULL CHECK (amount_cents >= 0),
    platform_fee_cents BIGINT NOT NULL CHECK (platform_fee_cents >= 0),
    seller_amount_cents BIGINT NOT NULL CHECK (seller_amount_cents >= 0),
    status             TEXT NOT NULL DEFAULT 'created'
                            CHECK (status IN ('created', 'paid', 'delivered', 'confirmed',
                                              'settled', 'disputed', 'refunded', 'cancelled')),
    auto_confirm_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_buyer ON orders (buyer_id);
CREATE INDEX IF NOT EXISTS idx_orders_seller ON orders (seller_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
-- One active order per (buyer, dataset) — prevent duplicate purchases.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_orders_active_per_buyer_dataset
    ON orders (buyer_id, dataset_id)
    WHERE status IN ('created', 'paid', 'delivered', 'confirmed', 'disputed');

CREATE TABLE IF NOT EXISTS payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL UNIQUE REFERENCES orders (id),
    channel         TEXT NOT NULL CHECK (channel IN ('wechat', 'alipay', 'stripe')),
    channel_txn_id  TEXT,                 -- unique once present (partial index below)
    amount_cents    BIGINT NOT NULL CHECK (amount_cents >= 0),
    status          TEXT NOT NULL DEFAULT 'created'
                         CHECK (status IN ('created', 'paid', 'refunded', 'refunding')),
    escrow_state    TEXT CHECK (escrow_state IN ('frozen', 'released', 'reverted')),
    idempotency_key TEXT UNIQUE,
    paid_at         TIMESTAMPTZ,
    raw_callback    JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_payments_channel_txn
    ON payments (channel_txn_id) WHERE channel_txn_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL UNIQUE REFERENCES orders (id),
    download_token_hash TEXT,
    expires_at          TIMESTAMPTZ,
    max_downloads       INTEGER NOT NULL DEFAULT 1,
    download_count      INTEGER NOT NULL DEFAULT 0,
    delivery_fingerprint TEXT,            -- buyer+order salted hash, for tracing
    last_download_ip    INET,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Split-settlement ledger (分账结算).
CREATE TABLE IF NOT EXISTS settlements (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL UNIQUE REFERENCES orders (id),
    split_txn_id        TEXT,             -- channel split id; unique once present
    platform_fee_cents  BIGINT NOT NULL CHECK (platform_fee_cents >= 0),
    seller_amount_cents BIGINT NOT NULL CHECK (seller_amount_cents >= 0),
    status              TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'success', 'failed', 'reverted')),
    idempotency_key     TEXT UNIQUE,      -- guards confirmed->settled against double-split
    executed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_settlements_split_txn
    ON settlements (split_txn_id) WHERE split_txn_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS disputes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders (id),
    raised_by       UUID NOT NULL REFERENCES users (id),
    reason          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                         CHECK (status IN ('open', 'reviewing', 'resolved_refund', 'resolved_release')),
    resolution_note TEXT,
    handled_by      UUID REFERENCES users (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_disputes_order ON disputes (order_id);

CREATE TABLE IF NOT EXISTS reviews (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id   UUID NOT NULL UNIQUE REFERENCES orders (id),
    dataset_id UUID NOT NULL REFERENCES datasets (id),
    buyer_id   UUID NOT NULL REFERENCES users (id),
    score      INTEGER NOT NULL CHECK (score BETWEEN 1 AND 5),
    comment    TEXT,
    issue_flag BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_reviews_dataset ON reviews (dataset_id);

-- ---------------------------------------------------------------------------
-- audit log — append-only (compliance). Updates/deletes are blocked by trigger.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id      UUID,
    actor_role    TEXT,
    action        TEXT NOT NULL,
    resource_type TEXT,
    resource_id   TEXT,
    ip            INET,
    user_agent    TEXT,
    detail        JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs (resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs (actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs (created_at);

CREATE FUNCTION audit_logs_block_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only: % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_logs_no_update
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_block_mutation();

COMMIT;
