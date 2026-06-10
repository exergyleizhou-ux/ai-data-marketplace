-- Allow the sandbox payment channel 'mock' (dev/test only) alongside the real
-- licensed channels. Production deployments simply never use it.
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_channel_check;
ALTER TABLE payments ADD CONSTRAINT payments_channel_check
    CHECK (channel IN ('wechat', 'alipay', 'stripe', 'mock'));
