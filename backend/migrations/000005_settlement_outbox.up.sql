-- Settlement outbox (H3): durable settlement jobs so a confirmed order always
-- gets settled even if the inline attempt fails (process crash, provider
-- blip). A background worker drains due rows under a PG advisory lock (so
-- multiple app instances don't double-process) and retries with backoff; the
-- settlements unique key remains the final double-split guard.
CREATE TABLE settlement_outbox (
    order_id        UUID PRIMARY KEY REFERENCES orders (id),
    status          TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'done', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hot path: "which pending jobs are due now?" — partial index on due rows.
CREATE INDEX idx_settlement_outbox_due
    ON settlement_outbox (next_attempt_at)
    WHERE status = 'pending';
