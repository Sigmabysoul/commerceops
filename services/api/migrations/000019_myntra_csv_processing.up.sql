ALTER TABLE processing_jobs
    ADD COLUMN upload_idempotency_key text,
    ADD COLUMN upload_request_hash text,
    ADD CONSTRAINT processing_jobs_upload_idempotency_pair CHECK (
        (upload_idempotency_key IS NULL AND upload_request_hash IS NULL) OR
        (upload_idempotency_key IS NOT NULL AND upload_request_hash IS NOT NULL AND
         btrim(upload_idempotency_key) <> '' AND length(upload_idempotency_key) <= 128 AND
         upload_request_hash ~ '^[0-9a-f]{64}$')
    );

CREATE UNIQUE INDEX processing_jobs_company_marketplace_upload_idempotency_unique
    ON processing_jobs(company_id, marketplace_key, upload_idempotency_key)
    WHERE upload_idempotency_key IS NOT NULL;

CREATE INDEX processing_jobs_myntra_claim_idx
    ON processing_jobs(created_at,id)
    WHERE marketplace_key='myntra'
      AND status IN ('queued','processing');
