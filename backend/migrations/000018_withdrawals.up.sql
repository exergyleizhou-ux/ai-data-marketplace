-- 000018: seller withdrawal requests (book-keeping; bank transfer is off-system).
CREATE TABLE IF NOT EXISTS withdrawal_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id       UUID NOT NULL REFERENCES users(id),
    amount_cents    BIGINT NOT NULL CHECK (amount_cents > 0),
    channel         TEXT NOT NULL,
    account_label   TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'approved', 'completed', 'rejected')),
    ops_note        TEXT,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ,
    processed_by    UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_withdrawals_seller
    ON withdrawal_requests (seller_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_withdrawals_pending
    ON withdrawal_requests (status, requested_at) WHERE status IN ('pending', 'approved');
