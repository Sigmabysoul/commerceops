-- Preserve physical jobs and their origin evidence: rollback is refused while
-- automation jobs exist. Never delete historical print/audit data to downgrade.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM printer_jobs WHERE origin_type='automation') THEN
  RAISE EXCEPTION 'Cannot downgrade while automation print jobs exist';
 END IF;
END $$;
DROP TABLE automation_executions;
DROP TABLE automation_domain_events;
DROP TABLE automation_rules;
DELETE FROM role_permissions WHERE permission_key IN ('automations.view','automations.manage');
DELETE FROM permissions WHERE key IN ('automations.view','automations.manage');
ALTER TABLE printer_jobs DROP CONSTRAINT printer_jobs_origin_type_check;
ALTER TABLE printer_jobs ADD CONSTRAINT printer_jobs_origin_type_check CHECK (origin_type IN ('ecommerce_batch','ecommerce_reprint','consignment','quick_print'));
ALTER TABLE companies DROP COLUMN timezone;
