ALTER TABLE employees ADD CONSTRAINT employees_company_id_id_unique UNIQUE (company_id,id);

CREATE TABLE worker_assignment_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces(key) ON DELETE RESTRICT,
    product_id uuid,
    employee_id uuid NOT NULL,
    priority integer NOT NULL DEFAULT 100 CHECK (priority BETWEEN 0 AND 10000),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    UNIQUE (company_id,id)
);
CREATE UNIQUE INDEX worker_assignment_rules_product_unique
    ON worker_assignment_rules(company_id,marketplace_key,product_id) WHERE product_id IS NOT NULL AND status='active';
CREATE UNIQUE INDEX worker_assignment_rules_default_unique
    ON worker_assignment_rules(company_id,marketplace_key) WHERE product_id IS NULL AND status='active';

CREATE TABLE batch_worker_assignments (
    company_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    product_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    assignment_rule_id uuid NOT NULL,
    total_quantity integer NOT NULL CHECK (total_quantity > 0),
    order_line_count integer NOT NULL CHECK (order_line_count > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (batch_id,product_id),
    FOREIGN KEY (company_id,batch_id) REFERENCES batches(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,assignment_rule_id) REFERENCES worker_assignment_rules(company_id,id) ON DELETE RESTRICT
);
CREATE INDEX batch_worker_assignments_company_batch_idx ON batch_worker_assignments(company_id,batch_id,employee_id);

ALTER TABLE print_jobs
    ADD COLUMN source_print_job_id uuid,
    ADD COLUMN reprint_reason text CHECK (reprint_reason IS NULL OR (reprint_reason=btrim(reprint_reason) AND reprint_reason<>'' AND length(reprint_reason)<=500)),
    ADD CONSTRAINT print_jobs_reprint_pair_check CHECK ((source_print_job_id IS NULL)=(reprint_reason IS NULL)),
    ADD CONSTRAINT print_jobs_source_print_job_fk FOREIGN KEY (company_id,source_print_job_id) REFERENCES print_jobs(company_id,id) ON DELETE RESTRICT;
CREATE INDEX print_jobs_company_source_idx ON print_jobs(company_id,source_print_job_id,created_at DESC) WHERE source_print_job_id IS NOT NULL;
