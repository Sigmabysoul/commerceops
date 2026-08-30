CREATE INDEX processing_jobs_amazon_claim_idx
    ON processing_jobs(created_at,id)
    WHERE marketplace_key='amazon'
      AND status IN ('queued','processing');
