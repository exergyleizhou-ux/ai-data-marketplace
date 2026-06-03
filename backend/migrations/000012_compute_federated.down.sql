-- Reverse 000012 (reverse order of creation).
ALTER TABLE dataset_compute_offers DROP COLUMN IF EXISTS allow_federated;
DROP INDEX IF EXISTS idx_compute_jobs_federated;
ALTER TABLE compute_jobs DROP COLUMN IF EXISTS federated_job_id;
DROP TABLE IF EXISTS compute_federated_jobs;
