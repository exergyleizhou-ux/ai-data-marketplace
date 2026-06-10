ALTER TABLE users DROP COLUMN IF EXISTS totp_secret, DROP COLUMN IF EXISTS totp_enabled; DROP TABLE IF EXISTS totp_recovery_codes, password_reset_tokens;
