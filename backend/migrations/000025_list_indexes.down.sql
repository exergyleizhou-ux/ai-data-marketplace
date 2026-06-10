BEGIN;
DROP INDEX IF EXISTS idx_orders_buyer_created;
DROP INDEX IF EXISTS idx_orders_seller_created;
CREATE INDEX IF NOT EXISTS idx_orders_buyer ON orders (buyer_id);
CREATE INDEX IF NOT EXISTS idx_orders_seller ON orders (seller_id);

DROP INDEX IF EXISTS idx_datasets_seller_created;
CREATE INDEX IF NOT EXISTS idx_datasets_seller ON datasets (seller_id);

DROP INDEX IF EXISTS idx_notifications_user_created;

DROP INDEX IF EXISTS idx_reviews_dataset_created;
CREATE INDEX IF NOT EXISTS idx_reviews_dataset ON reviews (dataset_id);
COMMIT;
