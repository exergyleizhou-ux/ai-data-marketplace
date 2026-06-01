-- Consent records: which legal document (and version) a user agreed to, and
-- when. Append-only — one row per agreement event — so that policy updates
-- which require re-consent leave a complete, auditable trail (PIPL/电商法 取证).
CREATE TABLE user_agreements (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id   UUID NOT NULL REFERENCES users (id),
    doc       TEXT NOT NULL,  -- e.g. 'terms', 'privacy', 'data_license'
    version   TEXT NOT NULL,  -- document version the user accepted, e.g. 'v1.0'
    agreed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "what has this user agreed to?" — latest-first lookup per user.
CREATE INDEX idx_user_agreements_user ON user_agreements (user_id, agreed_at DESC);
