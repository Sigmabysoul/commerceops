DROP TABLE IF EXISTS processing_errors;
DROP TABLE IF EXISTS marketplace_order_items;
DROP TABLE IF EXISTS marketplace_orders;
DROP TABLE IF EXISTS processing_jobs;
DROP TABLE IF EXISTS source_files;
DELETE FROM permissions WHERE key IN ('labels.upload','labels.process');
