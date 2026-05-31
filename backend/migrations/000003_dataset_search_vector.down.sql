DROP INDEX IF EXISTS idx_datasets_search;
ALTER TABLE datasets DROP COLUMN IF EXISTS search_vector;
