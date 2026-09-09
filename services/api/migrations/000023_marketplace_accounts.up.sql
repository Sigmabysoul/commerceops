-- Existing rows are retained as explicitly UNASSIGNED history. No seller
-- ownership is inferred from marketplace, workstation, SKU, or filenames.
CREATE TABLE business_identities (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
 display_name text NOT NULL CHECK (display_name=btrim(display_name) AND length(display_name) BETWEEN 1 AND 120),
 legal_name text CHECK (legal_name IS NULL OR length(legal_name) BETWEEN 1 AND 250),
 status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dormant','inactive')),
 legacy_unassigned boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(company_id,id),
 CHECK (NOT legacy_unassigned OR status='dormant')
);
CREATE TABLE marketplace_accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
 marketplace_key text NOT NULL REFERENCES marketplaces(key),
 business_identity_id uuid NOT NULL,
 internal_key text NOT NULL CHECK (internal_key ~ '^[a-z][a-z0-9_]{0,79}$'),
 display_name text NOT NULL CHECK (display_name=btrim(display_name) AND length(display_name) BETWEEN 1 AND 120),
 external_seller_id text CHECK (external_seller_id IS NULL OR length(external_seller_id) BETWEEN 1 AND 200),
 fulfillment_model text CHECK (fulfillment_model IS NULL OR length(fulfillment_model) BETWEEN 1 AND 80),
 integration_mode text NOT NULL CHECK (integration_mode IN ('manual_upload','api','file_watch','print_capture','partner_connector')),
 status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dormant','inactive')),
 metadata jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(metadata)='object' AND metadata - ARRAY['region','notes']::text[] = '{}'::jsonb AND octet_length(metadata::text)<=4096),
 legacy_unassigned boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(company_id,business_identity_id) REFERENCES business_identities(company_id,id),
 UNIQUE(company_id,id),
 UNIQUE(company_id,id,marketplace_key),
 UNIQUE(company_id,internal_key),
 CHECK (NOT legacy_unassigned OR status='dormant')
);
CREATE INDEX marketplace_accounts_browse_idx ON marketplace_accounts(company_id,marketplace_key,status);
INSERT INTO marketplaces(key,display_name) VALUES('jiomart','JioMart') ON CONFLICT(key) DO NOTHING;
INSERT INTO permissions(key,description) VALUES
 ('marketplace_accounts.view','View trading identities and marketplace seller accounts'),
 ('marketplace_accounts.manage','Manage trading identities and marketplace seller accounts');

CREATE TEMP TABLE legacy_account_scopes AS
 SELECT company_id,marketplace_key FROM sku_mappings
 UNION SELECT company_id,marketplace_key FROM source_files
 UNION SELECT company_id,marketplace_key FROM processing_jobs
 UNION SELECT company_id,marketplace_key FROM marketplace_orders
 UNION SELECT company_id,marketplace_key FROM batches;
INSERT INTO business_identities(company_id,display_name,status,legacy_unassigned)
 SELECT DISTINCT company_id,'Unassigned legacy identity','dormant',true FROM legacy_account_scopes;
INSERT INTO marketplace_accounts(company_id,marketplace_key,business_identity_id,internal_key,display_name,integration_mode,status,legacy_unassigned)
 SELECT s.company_id,s.marketplace_key,b.id,'legacy_unassigned_'||s.marketplace_key,'Unassigned legacy / '||s.marketplace_key,'manual_upload','dormant',true
 FROM legacy_account_scopes s JOIN business_identities b ON b.company_id=s.company_id AND b.legacy_unassigned;
DROP TABLE legacy_account_scopes;
ALTER TABLE sku_mappings ADD COLUMN marketplace_account_id uuid;
UPDATE sku_mappings t SET marketplace_account_id=a.id FROM marketplace_accounts a WHERE a.company_id=t.company_id AND a.marketplace_key=t.marketplace_key AND a.legacy_unassigned;
ALTER TABLE sku_mappings ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE sku_mappings ADD CONSTRAINT sku_mappings_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE sku_mappings ADD CONSTRAINT sku_mappings_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE source_files ADD COLUMN marketplace_account_id uuid;
UPDATE source_files t SET marketplace_account_id=a.id FROM marketplace_accounts a WHERE a.company_id=t.company_id AND a.marketplace_key=t.marketplace_key AND a.legacy_unassigned;
ALTER TABLE source_files ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE source_files ADD CONSTRAINT source_files_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE source_files ADD CONSTRAINT source_files_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE processing_jobs ADD COLUMN marketplace_account_id uuid;
UPDATE processing_jobs t SET marketplace_account_id=a.id FROM marketplace_accounts a WHERE a.company_id=t.company_id AND a.marketplace_key=t.marketplace_key AND a.legacy_unassigned;
ALTER TABLE processing_jobs ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE marketplace_orders ADD COLUMN marketplace_account_id uuid;
UPDATE marketplace_orders t SET marketplace_account_id=a.id FROM marketplace_accounts a WHERE a.company_id=t.company_id AND a.marketplace_key=t.marketplace_key AND a.legacy_unassigned;
ALTER TABLE marketplace_orders ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE batches ADD COLUMN marketplace_account_id uuid;
UPDATE batches t SET marketplace_account_id=a.id FROM marketplace_accounts a WHERE a.company_id=t.company_id AND a.marketplace_key=t.marketplace_key AND a.legacy_unassigned;
ALTER TABLE batches ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE batches ADD CONSTRAINT batches_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE batches ADD CONSTRAINT batches_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_source_account_fk FOREIGN KEY(company_id,source_file_id,marketplace_account_id) REFERENCES source_files(company_id,id,marketplace_account_id);
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_source_identity_unique UNIQUE(company_id,id,source_file_id,marketplace_account_id);
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_source_account_fk FOREIGN KEY(company_id,source_file_id,marketplace_account_id) REFERENCES source_files(company_id,id,marketplace_account_id);
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_job_source_account_fk FOREIGN KEY(company_id,processing_job_id,source_file_id,marketplace_account_id) REFERENCES processing_jobs(company_id,id,source_file_id,marketplace_account_id);
ALTER TABLE batch_members ADD COLUMN marketplace_account_id uuid;
UPDATE batch_members t SET marketplace_account_id=o.marketplace_account_id FROM marketplace_orders o WHERE o.company_id=t.company_id AND o.id=t.marketplace_order_id;
ALTER TABLE batch_members ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE batch_members ADD CONSTRAINT batch_members_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE cancellations ADD COLUMN marketplace_account_id uuid;
UPDATE cancellations t SET marketplace_account_id=o.marketplace_account_id FROM marketplace_orders o WHERE o.company_id=t.company_id AND o.id=t.marketplace_order_id;
ALTER TABLE cancellations ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE cancellations ADD CONSTRAINT cancellations_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE return_cases ADD COLUMN marketplace_account_id uuid;
UPDATE return_cases t SET marketplace_account_id=o.marketplace_account_id FROM marketplace_orders o WHERE o.company_id=t.company_id AND o.id=t.marketplace_order_id;
ALTER TABLE return_cases ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE return_cases ADD CONSTRAINT return_cases_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE marketplace_order_documents ADD COLUMN marketplace_account_id uuid;
UPDATE marketplace_order_documents t SET marketplace_account_id=o.marketplace_account_id FROM marketplace_orders o WHERE o.company_id=t.company_id AND o.id=t.order_id;
ALTER TABLE marketplace_order_documents ALTER COLUMN marketplace_account_id SET NOT NULL;
ALTER TABLE marketplace_order_documents ADD CONSTRAINT marketplace_order_documents_order_account_fk FOREIGN KEY(company_id,order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE batch_members ADD CONSTRAINT batch_members_batch_account_fk FOREIGN KEY(company_id,batch_id,marketplace_account_id) REFERENCES batches(company_id,id,marketplace_account_id);
ALTER TABLE marketplace_order_documents ADD CONSTRAINT marketplace_order_documents_source_account_fk FOREIGN KEY(company_id,source_file_id,marketplace_account_id) REFERENCES source_files(company_id,id,marketplace_account_id);
DROP INDEX sku_mappings_active_lookup_unique;
CREATE UNIQUE INDEX sku_mappings_active_lookup_unique ON sku_mappings(company_id,marketplace_account_id,sku) WHERE status='active';
ALTER TABLE source_files DROP CONSTRAINT source_files_company_id_marketplace_key_sha256_key;
ALTER TABLE source_files ADD CONSTRAINT source_files_account_hash_unique UNIQUE(company_id,marketplace_account_id,sha256);
DROP INDEX processing_jobs_company_marketplace_upload_idempotency_unique;
CREATE UNIQUE INDEX processing_jobs_company_account_upload_idempotency_unique ON processing_jobs(company_id,marketplace_account_id,upload_idempotency_key) WHERE upload_idempotency_key IS NOT NULL;
DROP INDEX marketplace_orders_awb_unique;
DROP INDEX marketplace_orders_order_id_unique;
CREATE UNIQUE INDEX marketplace_orders_awb_unique ON marketplace_orders(company_id,marketplace_account_id,awb) WHERE awb IS NOT NULL AND status<>'duplicate';
CREATE UNIQUE INDEX marketplace_orders_order_id_unique ON marketplace_orders(company_id,marketplace_account_id,marketplace_order_id) WHERE marketplace_order_id IS NOT NULL AND status<>'duplicate';
CREATE INDEX marketplace_orders_account_created_idx ON marketplace_orders(company_id,marketplace_account_id,created_at DESC);
CREATE INDEX processing_jobs_account_created_idx ON processing_jobs(company_id,marketplace_account_id,created_at DESC);
CREATE INDEX batches_account_created_idx ON batches(company_id,marketplace_account_id,created_at DESC);

-- Child scope may be copied only from a specific tenant-owned parent UUID.
-- The composite FKs still reject explicit disagreement. Never select an account
-- merely because it is the sole account for a marketplace.
CREATE FUNCTION inherit_marketplace_account() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_account uuid;
BEGIN
 IF TG_TABLE_NAME='processing_jobs' THEN
  SELECT marketplace_account_id INTO parent_account FROM source_files WHERE company_id=NEW.company_id AND id=NEW.source_file_id;
 ELSIF TG_TABLE_NAME='marketplace_orders' THEN
  SELECT marketplace_account_id INTO parent_account FROM processing_jobs WHERE company_id=NEW.company_id AND id=NEW.processing_job_id;
 ELSIF TG_TABLE_NAME='marketplace_order_documents' THEN
  SELECT marketplace_account_id INTO parent_account FROM marketplace_orders WHERE company_id=NEW.company_id AND id=NEW.order_id;
 ELSE
  SELECT marketplace_account_id INTO parent_account FROM marketplace_orders WHERE company_id=NEW.company_id AND id=NEW.marketplace_order_id;
 END IF;
 IF NEW.marketplace_account_id IS NULL THEN NEW.marketplace_account_id:=parent_account; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION immutable_marketplace_provenance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.company_id IS DISTINCT FROM OLD.company_id OR NEW.marketplace_account_id IS DISTINCT FROM OLD.marketplace_account_id THEN
  RAISE EXCEPTION 'marketplace account provenance is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION guard_marketplace_account_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.company_id IS DISTINCT FROM OLD.company_id OR NEW.legacy_unassigned IS DISTINCT FROM OLD.legacy_unassigned OR OLD.legacy_unassigned THEN
  RAISE EXCEPTION 'tenant and legacy identity cannot be changed' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='marketplace_accounts' AND to_jsonb(NEW)->>'marketplace_key' IS DISTINCT FROM to_jsonb(OLD)->>'marketplace_key' THEN
  RAISE EXCEPTION 'account marketplace cannot be changed' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER business_identities_guard BEFORE UPDATE ON business_identities FOR EACH ROW EXECUTE FUNCTION guard_marketplace_account_identity();
CREATE TRIGGER marketplace_accounts_guard BEFORE UPDATE ON marketplace_accounts FOR EACH ROW EXECUTE FUNCTION guard_marketplace_account_identity();
CREATE TRIGGER processing_jobs_inherit_account BEFORE INSERT ON processing_jobs FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER marketplace_orders_inherit_account BEFORE INSERT ON marketplace_orders FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER batch_members_inherit_account BEFORE INSERT ON batch_members FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER cancellations_inherit_account BEFORE INSERT ON cancellations FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER return_cases_inherit_account BEFORE INSERT ON return_cases FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER marketplace_order_documents_inherit_account BEFORE INSERT ON marketplace_order_documents FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account();
CREATE TRIGGER sku_mappings_immutable_account BEFORE UPDATE ON sku_mappings FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER source_files_immutable_account BEFORE UPDATE ON source_files FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER processing_jobs_immutable_account BEFORE UPDATE ON processing_jobs FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER marketplace_orders_immutable_account BEFORE UPDATE ON marketplace_orders FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER batches_immutable_account BEFORE UPDATE ON batches FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER batch_members_immutable_account BEFORE UPDATE ON batch_members FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER cancellations_immutable_account BEFORE UPDATE ON cancellations FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER return_cases_immutable_account BEFORE UPDATE ON return_cases FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
CREATE TRIGGER marketplace_order_documents_immutable_account BEFORE UPDATE ON marketplace_order_documents FOR EACH ROW EXECUTE FUNCTION immutable_marketplace_provenance();
