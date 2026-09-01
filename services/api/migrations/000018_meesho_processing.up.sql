CREATE INDEX processing_jobs_meesho_claim_idx
    ON processing_jobs(created_at,id)
    WHERE marketplace_key='meesho'
      AND status IN ('queued','processing');
