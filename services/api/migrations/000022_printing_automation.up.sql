ALTER TABLE companies ADD COLUMN timezone text NOT NULL DEFAULT 'UTC' CHECK (length(timezone) BETWEEN 1 AND 100);
ALTER TABLE printer_jobs DROP CONSTRAINT printer_jobs_origin_type_check;
ALTER TABLE printer_jobs ADD CONSTRAINT printer_jobs_origin_type_check CHECK (origin_type IN ('ecommerce_batch','ecommerce_reprint','consignment','quick_print','automation'));
CREATE TABLE automation_rules (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
 name text NOT NULL CHECK (name=btrim(name) AND length(name) BETWEEN 1 AND 120),
 enabled boolean NOT NULL DEFAULT false,
 paused boolean NOT NULL DEFAULT false,
 trigger_type text NOT NULL CHECK (trigger_type IN ('scheduled','ecommerce_batch_ready','consignment_packing','consignment_packed')),
 schedule jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(schedule)='object'),
 timezone text NOT NULL,
 asset_id uuid NOT NULL,
 printer_id uuid NOT NULL,
 copies integer NOT NULL CHECK (copies BETWEEN 1 AND 100),
 daily_limit integer CHECK (daily_limit BETWEEN 1 AND 10000),
 failure_threshold integer NOT NULL CHECK (failure_threshold BETWEEN 1 AND 20),
 backoff_seconds integer NOT NULL CHECK (backoff_seconds BETWEEN 1 AND 3600),
 consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures>=0),
 backoff_until timestamptz,
 created_by uuid NOT NULL,
 version integer NOT NULL DEFAULT 1 CHECK (version>0),
 next_run_at timestamptz,
 activated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(company_id,id),
 FOREIGN KEY(company_id,asset_id) REFERENCES print_library_assets(company_id,id),
 FOREIGN KEY(company_id,printer_id) REFERENCES registered_printers(company_id,id),
 FOREIGN KEY(company_id,created_by) REFERENCES company_users(company_id,user_id)
);
CREATE INDEX automation_rules_due_idx ON automation_rules(next_run_at) WHERE enabled AND NOT paused AND trigger_type='scheduled';
CREATE TABLE automation_domain_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 company_id uuid NOT NULL REFERENCES companies(id),
 event_type text NOT NULL CHECK (event_type IN ('ecommerce_batch_ready','consignment_packing','consignment_packed')),
 source_id uuid NOT NULL,
 source_version integer NOT NULL CHECK (source_version>0),
 actor_user_id uuid NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 processed_at timestamptz,
 UNIQUE(company_id,id),
 UNIQUE(company_id,event_type,source_id,source_version),
 FOREIGN KEY(company_id,actor_user_id) REFERENCES company_users(company_id,user_id)
);
CREATE INDEX automation_events_pending_idx ON automation_domain_events(occurred_at) WHERE processed_at IS NULL;
CREATE TABLE automation_executions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 company_id uuid NOT NULL,
 rule_id uuid NOT NULL,
 rule_version integer NOT NULL,
 occurrence_key text NOT NULL CHECK (length(occurrence_key) BETWEEN 1 AND 180),
 event_id uuid,
 scheduled_at timestamptz,
 test_run boolean NOT NULL DEFAULT false,
 snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object'),
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','skipped')),
 attempt_count integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(),
 lease_token uuid,
 lease_expires_at timestamptz,
 error text,
 printer_job_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(company_id,id),
 UNIQUE(company_id,rule_id,occurrence_key),
 UNIQUE(company_id,printer_job_id),
 FOREIGN KEY(company_id,rule_id) REFERENCES automation_rules(company_id,id),
 FOREIGN KEY(company_id,event_id) REFERENCES automation_domain_events(company_id,id),
 FOREIGN KEY(company_id,printer_job_id) REFERENCES printer_jobs(company_id,id),
 CHECK ((lease_token IS NULL)=(lease_expires_at IS NULL)),
 CHECK ((status='completed')=(printer_job_id IS NOT NULL))
);
CREATE INDEX automation_executions_claim_idx ON automation_executions(available_at,created_at) WHERE status IN ('pending','running','failed');
CREATE INDEX automation_executions_history_idx ON automation_executions(company_id,rule_id,created_at DESC);
INSERT INTO permissions(key,description) VALUES ('automations.view','View printing automation rules, runs and reports'),('automations.manage','Manage and test printing automations');
