-- Snapshot the offer's output-gate config onto each job at submit time, so a
-- later seller edit of the offer can't retroactively change a queued/running
-- job's review/size behavior (config TOCTOU; audit findings #6/#7). Nullable and
-- backward-compatible: rows created before this migration stay NULL and the
-- worker falls back to the live offer. Mirrors the dp_epsilon snapshot precedent.
ALTER TABLE compute_jobs ADD COLUMN review_output boolean;
ALTER TABLE compute_jobs ADD COLUMN max_output_bytes bigint;
