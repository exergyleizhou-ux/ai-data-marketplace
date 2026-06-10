DROP INDEX IF EXISTS uniq_compute_entitlements_order;
DROP INDEX IF EXISTS uniq_orders_active_per_buyer_dataset;
CREATE UNIQUE INDEX uniq_orders_active_per_buyer_dataset
    ON orders (buyer_id, dataset_id)
    WHERE status IN ('created', 'paid', 'delivered', 'confirmed', 'disputed');
ALTER TABLE orders ALTER COLUMN version_id SET NOT NULL;
ALTER TABLE orders DROP COLUMN IF EXISTS product_type;
