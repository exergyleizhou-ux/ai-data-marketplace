-- Full-text search column for the dataset catalog. Populated by the app with
-- space-joined Chinese word tokens (see platform/textseg) under the 'simple'
-- config, then GIN-indexed for ranked keyword search (docs §6.4).
ALTER TABLE datasets ADD COLUMN search_vector tsvector;
CREATE INDEX idx_datasets_search ON datasets USING GIN (search_vector);
