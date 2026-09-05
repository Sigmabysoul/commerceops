DROP INDEX IF EXISTS inventory_transactions_company_created_idx;
DROP INDEX IF EXISTS print_jobs_company_status_completed_idx;
DROP INDEX IF EXISTS processing_jobs_company_status_created_idx;
DROP INDEX IF EXISTS marketplace_orders_company_status_created_idx;
DROP INDEX IF EXISTS marketplace_orders_company_marketplace_created_idx;
DELETE FROM permissions WHERE key='reports.view';
