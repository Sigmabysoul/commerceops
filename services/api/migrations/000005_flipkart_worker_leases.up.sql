ALTER TABLE processing_jobs
    ADD COLUMN worker_id text,
    ADD COLUMN lease_expires_at timestamptz,
    ADD CONSTRAINT processing_jobs_lease_pair_check CHECK (
        (worker_id IS NULL AND lease_expires_at IS NULL)
        OR
        (worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND char_length(worker_id) BETWEEN 1 AND 128)
    );

-- Protect work started by the pre-lease application during a rolling deploy.
-- These compatibility leases expire automatically if the old process does not
-- finish before a lease-aware worker is allowed to reclaim the job.
UPDATE processing_jobs
SET worker_id = 'migration-' || gen_random_uuid()::text,
    lease_expires_at = now() + interval '2 minutes'
WHERE marketplace_key = 'flipkart'
  AND status = 'processing';

CREATE INDEX processing_jobs_flipkart_claim_idx
    ON processing_jobs(created_at, id)
    WHERE marketplace_key = 'flipkart'
      AND status IN ('queued', 'processing');
