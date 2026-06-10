-- Datasheet (Gebru et al. 2021 / HF dataset cards): structured documentation a
-- seller can attach to a dataset — intended uses, composition, collection,
-- limitations, etc. Optional, JSONB (like source_declaration), editable anytime.
ALTER TABLE datasets ADD COLUMN IF NOT EXISTS datasheet JSONB;
