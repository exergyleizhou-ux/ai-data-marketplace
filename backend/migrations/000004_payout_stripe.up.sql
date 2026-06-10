-- Allow 'stripe' as a payout (split-receiving) channel so Stripe Connect
-- connected-account ids can be persisted in payout_accounts (docs §2.1, H1).
-- Also enforce one payout account per (user, channel) so the seller->account
-- mapping is a clean upsert instead of an in-memory cache.
ALTER TABLE payout_accounts DROP CONSTRAINT IF EXISTS payout_accounts_channel_check;
ALTER TABLE payout_accounts ADD CONSTRAINT payout_accounts_channel_check
    CHECK (channel IN ('wechat', 'alipay', 'stripe'));

CREATE UNIQUE INDEX IF NOT EXISTS uniq_payout_accounts_user_channel
    ON payout_accounts (user_id, channel);
