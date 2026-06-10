BEGIN;
-- Stored so a retried POST /payments returns the SAME pay URL / client secret
-- as the first call, instead of minting a new provider charge (which made the
-- DB row's channel_txn_id diverge from what the buyer actually pays — webhook
-- would then never match and the order stuck unpaid).
ALTER TABLE payments ADD COLUMN IF NOT EXISTS pay_url TEXT;
COMMIT;
