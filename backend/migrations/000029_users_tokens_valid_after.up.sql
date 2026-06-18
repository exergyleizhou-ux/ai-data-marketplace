-- Session-invalidation epoch: refresh tokens issued before this instant are
-- rejected. Set on password reset (and available for forced logout) so a reset
-- terminates pre-existing sessions. NULL = never invalidated. Access tokens are
-- short-lived and expire on their own TTL, so only the refresh path checks this.
ALTER TABLE users ADD COLUMN tokens_valid_after timestamptz;
