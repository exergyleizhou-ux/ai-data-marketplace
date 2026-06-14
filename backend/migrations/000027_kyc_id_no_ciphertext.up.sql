-- Reversible at-rest encryption of the raw ID number for lawful retrieval.
-- id_no_hash stays as the blind index (HMAC, equality/dedup). id_no_ciphertext
-- holds AES-256-GCM(nonce‖ciphertext, AAD=user_id), decryptable only via the
-- ops-gated, audited reveal path. NULL for company KYC / legacy rows.
ALTER TABLE kyc_records ADD COLUMN id_no_ciphertext BYTEA;
