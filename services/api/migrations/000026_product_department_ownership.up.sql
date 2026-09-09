-- Product ownership reuses Consignment's canonical departments. History is
-- effective-dated so a new assignment never rewrites prior operational work.
CREATE TABLE product_department_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    product_id uuid NOT NULL,
    department_id uuid NOT NULL,
    assigned_by uuid NOT NULL,
    effective_from timestamptz NOT NULL DEFAULT now(),
    effective_to timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,department_id) REFERENCES consignment_departments(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,assigned_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);
CREATE UNIQUE INDEX product_department_assignments_one_active
    ON product_department_assignments(company_id,product_id) WHERE effective_to IS NULL;
CREATE INDEX product_department_assignments_history
    ON product_department_assignments(company_id,product_id,effective_from DESC);
