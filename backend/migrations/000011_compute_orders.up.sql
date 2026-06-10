-- Compute (C2D) orders: a buyer pays for a compute entitlement through the SAME
-- order + payment + settlement pipeline as a download. Tag the product type and
-- allow a NULL version (a compute order doesn't reference a dataset version).
ALTER TABLE orders ADD COLUMN IF NOT EXISTS product_type TEXT NOT NULL DEFAULT 'download'
    CHECK (product_type IN ('download', 'compute'));
ALTER TABLE orders ALTER COLUMN version_id DROP NOT NULL;

-- Make the duplicate-active-order guard per product type, so a buyer may hold a
-- download order AND a compute order for the same dataset at the same time
-- (still at most one active order of each kind).
DROP INDEX uniq_orders_active_per_buyer_dataset;
CREATE UNIQUE INDEX IF NOT EXISTS uniq_orders_active_per_buyer_dataset
    ON orders (buyer_id, dataset_id, product_type)
    WHERE status IN ('created', 'paid', 'delivered', 'confirmed', 'disputed');

-- At most one compute entitlement per order, so granting on payment is
-- idempotent (a retried webhook can't double-grant).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_compute_entitlements_order
    ON compute_entitlements (order_id) WHERE order_id IS NOT NULL;
