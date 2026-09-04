CREATE INDEX processing_jobs_snapdeal_claim_idx
    ON processing_jobs(created_at,id)
    WHERE marketplace_key='snapdeal'
      AND status IN ('queued','processing');
