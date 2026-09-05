INSERT INTO permissions(key,description) VALUES
 ('reports.view','View operational dashboards and reports')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;

CREATE INDEX marketplace_orders_company_marketplace_created_idx ON marketplace_orders(company_id,marketplace_key,created_at DESC);
CREATE INDEX marketplace_orders_company_status_created_idx ON marketplace_orders(company_id,status,created_at DESC);
CREATE INDEX processing_jobs_company_status_created_idx ON processing_jobs(company_id,status,created_at DESC);
CREATE INDEX print_jobs_company_status_completed_idx ON print_jobs(company_id,status,completed_at DESC) WHERE completed_at IS NOT NULL;
CREATE INDEX inventory_transactions_company_created_idx ON inventory_transactions(company_id,created_at DESC,id DESC);
