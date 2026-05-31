-- Reverse 000004. Drop any stripe rows first so the narrowed CHECK can re-apply.
DROP INDEX IF EXISTS uniq_payout_accounts_user_channel;
DELETE FROM payout_accounts WHERE channel = 'stripe';
ALTER TABLE payout_accounts DROP CONSTRAINT payout_accounts_channel_check;
ALTER TABLE payout_accounts ADD CONSTRAINT payout_accounts_channel_check
    CHECK (channel IN ('wechat', 'alipay'));
