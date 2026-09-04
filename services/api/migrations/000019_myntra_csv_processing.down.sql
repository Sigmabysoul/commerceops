DROP INDEX IF EXISTS processing_jobs_myntra_claim_idx;
DROP INDEX IF EXISTS processing_jobs_company_marketplace_upload_idempotency_unique;
ALTER TABLE processing_jobs
    DROP CONSTRAINT IF EXISTS processing_jobs_upload_idempotency_pair,
    DROP COLUMN IF EXISTS upload_request_hash,
    DROP COLUMN IF EXISTS upload_idempotency_key;
