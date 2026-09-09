-- Preserve historical marketplace records without inventing seller ownership.
ALTER TABLE business_identities ADD COLUMN legacy_unassigned boolean NOT NULL DEFAULT false;
ALTER TABLE marketplace_accounts ADD COLUMN legacy_unassigned boolean NOT NULL DEFAULT false;
ALTER TABLE business_identities ADD CONSTRAINT business_identities_legacy_status CHECK (NOT legacy_unassigned OR status = 'dormant');
ALTER TABLE marketplace_accounts ADD CONSTRAINT marketplace_accounts_legacy_status CHECK (NOT legacy_unassigned OR status = 'dormant');

CREATE TEMP TABLE legacy_account_scopes AS
    SELECT company_id, marketplace_key FROM sku_mappings
    UNION SELECT company_id, marketplace_key FROM source_files
    UNION SELECT company_id, marketplace_key FROM processing_jobs
    UNION SELECT company_id, marketplace_key FROM marketplace_orders;
INSERT INTO business_identities(company_id,display_name,status,legacy_unassigned)
    SELECT DISTINCT company_id, 'Unassigned legacy identity', 'dormant', true FROM legacy_account_scopes;
INSERT INTO marketplace_accounts(company_id,marketplace_key,business_identity_id,internal_key,display_name,integration_mode,status,legacy_unassigned)
    SELECT scope.company_id,scope.marketplace_key,identity.id,'legacy_unassigned_' || scope.marketplace_key,'Unassigned legacy / ' || scope.marketplace_key,'manual_upload','dormant',true
    FROM legacy_account_scopes scope
    JOIN business_identities identity ON identity.company_id=scope.company_id AND identity.legacy_unassigned;
DROP TABLE legacy_account_scopes;

ALTER TABLE sku_mappings ADD COLUMN marketplace_account_id uuid;
ALTER TABLE source_files ADD COLUMN marketplace_account_id uuid;
ALTER TABLE processing_jobs ADD COLUMN marketplace_account_id uuid;
ALTER TABLE marketplace_orders ADD COLUMN marketplace_account_id uuid;
UPDATE sku_mappings record SET marketplace_account_id=account.id FROM marketplace_accounts account WHERE account.company_id=record.company_id AND account.marketplace_key=record.marketplace_key AND account.legacy_unassigned;
UPDATE source_files record SET marketplace_account_id=account.id FROM marketplace_accounts account WHERE account.company_id=record.company_id AND account.marketplace_key=record.marketplace_key AND account.legacy_unassigned;
UPDATE processing_jobs record SET marketplace_account_id=source.marketplace_account_id FROM source_files source WHERE source.company_id=record.company_id AND source.id=record.source_file_id;
UPDATE marketplace_orders record SET marketplace_account_id=job.marketplace_account_id FROM processing_jobs job WHERE job.company_id=record.company_id AND job.id=record.processing_job_id;

ALTER TABLE sku_mappings ADD CONSTRAINT sku_mappings_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE source_files ADD CONSTRAINT source_files_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);

DROP INDEX sku_mappings_active_lookup_unique;
CREATE UNIQUE INDEX sku_mappings_active_account_lookup_unique ON sku_mappings(company_id,marketplace_account_id,sku) WHERE status='active';
CREATE UNIQUE INDEX sku_mappings_active_legacy_lookup_unique ON sku_mappings(company_id,marketplace_key,sku) WHERE status='active' AND marketplace_account_id IS NULL;
ALTER TABLE source_files DROP CONSTRAINT source_files_company_id_marketplace_key_sha256_key;
ALTER TABLE source_files ADD CONSTRAINT source_files_account_hash_unique UNIQUE(company_id,marketplace_account_id,sha256);
CREATE UNIQUE INDEX source_files_legacy_hash_unique ON source_files(company_id,marketplace_key,sha256) WHERE marketplace_account_id IS NULL;
DROP INDEX processing_jobs_company_marketplace_upload_idempotency_unique;
CREATE UNIQUE INDEX processing_jobs_company_account_upload_idempotency_unique ON processing_jobs(company_id,marketplace_account_id,upload_idempotency_key) WHERE upload_idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX processing_jobs_company_legacy_upload_idempotency_unique ON processing_jobs(company_id,marketplace_key,upload_idempotency_key) WHERE upload_idempotency_key IS NOT NULL AND marketplace_account_id IS NULL;
DROP INDEX marketplace_orders_awb_unique;
DROP INDEX marketplace_orders_order_id_unique;
CREATE UNIQUE INDEX marketplace_orders_account_awb_unique ON marketplace_orders(company_id,marketplace_account_id,awb) WHERE awb IS NOT NULL AND status <> 'duplicate';
CREATE UNIQUE INDEX marketplace_orders_account_order_id_unique ON marketplace_orders(company_id,marketplace_account_id,marketplace_order_id) WHERE marketplace_order_id IS NOT NULL AND status <> 'duplicate';
CREATE UNIQUE INDEX marketplace_orders_legacy_awb_unique ON marketplace_orders(company_id,marketplace_key,awb) WHERE marketplace_account_id IS NULL AND awb IS NOT NULL AND status <> 'duplicate';
CREATE UNIQUE INDEX marketplace_orders_legacy_order_id_unique ON marketplace_orders(company_id,marketplace_key,marketplace_order_id) WHERE marketplace_account_id IS NULL AND marketplace_order_id IS NOT NULL AND status <> 'duplicate';
