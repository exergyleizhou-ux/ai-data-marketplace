BEGIN;
-- Composite (filter, sort) indexes for the hot paginated lists, which all
-- run `WHERE col = $1 ORDER BY created_at DESC LIMIT/OFFSET`. The previous
-- single-column indexes satisfied the WHERE but forced a sort of every
-- matching row; these serve the rows pre-ordered. The strictly-redundant
-- single-column indexes are dropped (the composite's leading column covers
-- the FK / equality lookups).
CREATE INDEX IF NOT EXISTS idx_orders_buyer_created
    ON orders (buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_seller_created
    ON orders (seller_id, created_at DESC);
DROP INDEX IF EXISTS idx_orders_buyer;
DROP INDEX IF EXISTS idx_orders_seller;

CREATE INDEX IF NOT EXISTS idx_datasets_seller_created
    ON datasets (seller_id, created_at DESC);
DROP INDEX IF EXISTS idx_datasets_seller;

-- The existing partial index (user_id, created_at DESC) WHERE is_read=false
-- only serves the unread view; the main inbox lists ALL notifications.
CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_reviews_dataset_created
    ON reviews (dataset_id, created_at DESC);
DROP INDEX IF EXISTS idx_reviews_dataset;
COMMIT;
