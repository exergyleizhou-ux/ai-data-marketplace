-- Self-serve API keys for the Oasis Verify product (the metered, billable surface
-- of the verification API). Keys are stored only as a SHA-256 hash; the plaintext
-- is returned once at issue time. Monthly usage is metered per key against its
-- tier quota.
CREATE TABLE api_keys (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL DEFAULT '',
    prefix       text NOT NULL,                 -- "sk_live_xxxxxxxx", safe to display
    key_hash     text NOT NULL UNIQUE,          -- sha256(plaintext); lookups compare hashes
    tier         text NOT NULL DEFAULT 'free',
    usage_month  text NOT NULL DEFAULT '',      -- 'YYYY-MM' the counter belongs to
    usage_count  integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);

CREATE INDEX api_keys_account_idx ON api_keys (account_id);
