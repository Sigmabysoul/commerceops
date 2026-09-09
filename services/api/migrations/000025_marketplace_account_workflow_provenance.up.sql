-- Phase 16 extends account provenance to operational workflow records without
-- changing Inventory authority or inferring ownership from workers/devices.
ALTER TABLE batches ADD COLUMN marketplace_account_id uuid;
ALTER TABLE batch_members ADD COLUMN marketplace_account_id uuid;
ALTER TABLE cancellations ADD COLUMN marketplace_account_id uuid;
ALTER TABLE return_cases ADD COLUMN marketplace_account_id uuid;
ALTER TABLE marketplace_order_documents ADD COLUMN marketplace_account_id uuid;

-- Composite foreign keys below bind a workflow record to the same account as
-- its immutable parent record. PostgreSQL requires these referenced keys to be
-- unique even though `id` is already the primary key.
ALTER TABLE marketplace_orders ADD CONSTRAINT marketplace_orders_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);
ALTER TABLE batches ADD CONSTRAINT batches_account_identity_unique UNIQUE(company_id,id,marketplace_account_id);

UPDATE batches batch SET marketplace_account_id=account.id
FROM marketplace_accounts account
WHERE account.company_id=batch.company_id AND account.marketplace_key=batch.marketplace_key AND account.legacy_unassigned;
UPDATE batch_members member SET marketplace_account_id=orders.marketplace_account_id
FROM marketplace_orders orders
WHERE orders.company_id=member.company_id AND orders.id=member.marketplace_order_id;
UPDATE cancellations cancellation SET marketplace_account_id=orders.marketplace_account_id
FROM marketplace_orders orders
WHERE orders.company_id=cancellation.company_id AND orders.id=cancellation.marketplace_order_id;
UPDATE return_cases return_case SET marketplace_account_id=orders.marketplace_account_id
FROM marketplace_orders orders
WHERE orders.company_id=return_case.company_id AND orders.id=return_case.marketplace_order_id;
UPDATE marketplace_order_documents document SET marketplace_account_id=orders.marketplace_account_id
FROM marketplace_orders orders
WHERE orders.company_id=document.company_id AND orders.id=document.order_id;

ALTER TABLE batches ADD CONSTRAINT batches_account_marketplace_fk FOREIGN KEY(company_id,marketplace_account_id,marketplace_key) REFERENCES marketplace_accounts(company_id,id,marketplace_key);
ALTER TABLE batch_members ADD CONSTRAINT batch_members_batch_account_fk FOREIGN KEY(company_id,batch_id,marketplace_account_id) REFERENCES batches(company_id,id,marketplace_account_id);
ALTER TABLE batch_members ADD CONSTRAINT batch_members_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE cancellations ADD CONSTRAINT cancellations_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE return_cases ADD CONSTRAINT return_cases_order_account_fk FOREIGN KEY(company_id,marketplace_order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);
ALTER TABLE marketplace_order_documents ADD CONSTRAINT marketplace_order_documents_order_account_fk FOREIGN KEY(company_id,order_id,marketplace_account_id) REFERENCES marketplace_orders(company_id,id,marketplace_account_id);

CREATE INDEX batches_account_created_idx ON batches(company_id,marketplace_account_id,created_at DESC);

CREATE FUNCTION inherit_marketplace_account_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE inherited_account uuid;
BEGIN
  IF TG_TABLE_NAME='batch_members' OR TG_TABLE_NAME='cancellations' OR TG_TABLE_NAME='return_cases' THEN
    SELECT marketplace_account_id INTO inherited_account FROM marketplace_orders WHERE company_id=NEW.company_id AND id=NEW.marketplace_order_id;
  ELSE
    SELECT marketplace_account_id INTO inherited_account FROM marketplace_orders WHERE company_id=NEW.company_id AND id=NEW.order_id;
  END IF;
  IF NEW.marketplace_account_id IS NULL THEN NEW.marketplace_account_id := inherited_account; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER batch_members_inherit_marketplace_account BEFORE INSERT ON batch_members FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account_workflow();
CREATE TRIGGER cancellations_inherit_marketplace_account BEFORE INSERT ON cancellations FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account_workflow();
CREATE TRIGGER return_cases_inherit_marketplace_account BEFORE INSERT ON return_cases FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account_workflow();
CREATE TRIGGER marketplace_order_documents_inherit_marketplace_account BEFORE INSERT ON marketplace_order_documents FOR EACH ROW EXECUTE FUNCTION inherit_marketplace_account_workflow();
