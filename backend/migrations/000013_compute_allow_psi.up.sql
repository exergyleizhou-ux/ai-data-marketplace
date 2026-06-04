-- Direction D 阶段2: a dedicated seller consent for private set intersection (PSI).
-- Reusing allow_federated conflated two distinct privacy exposures — co-training a
-- model vs. revealing set overlap. A seller may consent to one but not the other,
-- so PSI gets its own flag. Nullable-or-defaulted column add (safe online).
ALTER TABLE dataset_compute_offers ADD COLUMN allow_psi BOOLEAN NOT NULL DEFAULT false;
