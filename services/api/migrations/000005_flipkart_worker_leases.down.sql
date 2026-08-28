DROP INDEX IF EXISTS processing_jobs_flipkart_claim_idx;

ALTER TABLE processing_jobs
    DROP CONSTRAINT IF EXISTS processing_jobs_lease_pair_check,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS worker_id;
