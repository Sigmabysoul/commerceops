ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction','ecommerce_out','return_restock','consignment_out'));

CREATE TABLE consignment_departments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (name=btrim(name) AND name<>'' AND length(name)<=100),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,created_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id)
);
CREATE UNIQUE INDEX consignment_departments_company_name_idx ON consignment_departments(company_id,lower(name));

CREATE TABLE consignment_department_members (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    department_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    assigned_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,department_id) REFERENCES consignment_departments(company_id,id) ON DELETE CASCADE,
    FOREIGN KEY (company_id,employee_id) REFERENCES employees(company_id,id) ON DELETE CASCADE,
    FOREIGN KEY (company_id,assigned_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,department_id,employee_id)
);
CREATE INDEX consignment_department_members_employee_idx ON consignment_department_members(company_id,employee_id,department_id);

CREATE TABLE consignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    order_reference text NOT NULL CHECK (order_reference=btrim(order_reference) AND order_reference<>'' AND length(order_reference)<=200),
    dealer_reference text CHECK (dealer_reference IS NULL OR (dealer_reference=btrim(dealer_reference) AND dealer_reference<>'' AND length(dealer_reference)<=200)),
    pouch_reference text CHECK (pouch_reference IS NULL OR (pouch_reference=btrim(pouch_reference) AND pouch_reference<>'' AND length(pouch_reference)<=200)),
    source_type text NOT NULL CHECK (source_type IN ('manual','import')),
    source_reference text CHECK (source_reference IS NULL OR (source_reference=btrim(source_reference) AND source_reference<>'' AND length(source_reference)<=500)),
    status text NOT NULL DEFAULT 'created' CHECK (status IN ('created','allocated','picking','ready','packing','packed','outbound','completed','cancelled')),
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    created_by uuid NOT NULL,
    allocated_by uuid,
    outbound_by uuid,
    completed_by uuid,
    cancelled_by uuid,
    allocated_at timestamptz,
    outbound_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    version integer NOT NULL DEFAULT 1 CHECK (version>0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,created_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,allocated_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,outbound_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,completed_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,cancelled_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,order_reference),
    UNIQUE (company_id,idempotency_key),
    CHECK ((source_type='manual') OR source_reference IS NOT NULL),
    CHECK ((allocated_by IS NULL)=(allocated_at IS NULL)),
    CHECK ((outbound_by IS NULL)=(outbound_at IS NULL)),
    CHECK ((completed_by IS NULL)=(completed_at IS NULL)),
    CHECK ((cancelled_by IS NULL)=(cancelled_at IS NULL))
);
CREATE INDEX consignments_company_status_created_idx ON consignments(company_id,status,created_at DESC,id DESC);
CREATE INDEX consignments_company_pouch_idx ON consignments(company_id,pouch_reference) WHERE pouch_reference IS NOT NULL;

CREATE TABLE consignment_lines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    consignment_id uuid NOT NULL,
    product_id uuid NOT NULL,
    department_id uuid NOT NULL,
    required_quantity bigint NOT NULL CHECK (required_quantity>0),
    ready_quantity bigint NOT NULL DEFAULT 0 CHECK (ready_quantity>=0),
    packed_quantity bigint NOT NULL DEFAULT 0 CHECK (packed_quantity>=0),
    version integer NOT NULL DEFAULT 1 CHECK (version>0),
    updated_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,consignment_id) REFERENCES consignments(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,department_id) REFERENCES consignment_departments(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,updated_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,consignment_id,product_id,department_id),
    CHECK (packed_quantity<=ready_quantity AND ready_quantity<=required_quantity)
);
CREATE INDEX consignment_lines_company_department_idx ON consignment_lines(company_id,department_id,consignment_id);
CREATE INDEX consignment_lines_company_product_idx ON consignment_lines(company_id,product_id,consignment_id);

CREATE TABLE consignment_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    consignment_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('created','allocated','status_changed','line_progress','outbound','completed','cancelled')),
    actor_user_id uuid NOT NULL,
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object'),
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,consignment_id) REFERENCES consignments(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,actor_user_id) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,idempotency_key)
);
CREATE INDEX consignment_events_company_consignment_idx ON consignment_events(company_id,consignment_id,created_at,id);
CREATE INDEX consignment_events_company_type_idx ON consignment_events(company_id,event_type,created_at,id);

CREATE FUNCTION protect_consignment_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'consignment event history is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER consignment_events_immutable BEFORE UPDATE OR DELETE ON consignment_events
    FOR EACH ROW EXECUTE FUNCTION protect_consignment_events();

CREATE UNIQUE INDEX inventory_transactions_consignment_source_idx
    ON inventory_transactions(company_id,reference_type,reference_id,product_id)
    WHERE transaction_type='consignment_out';

INSERT INTO permissions(key,description) VALUES
    ('consignments.view','View all consignment work'),
    ('consignments.work','View and update assigned consignment departments'),
    ('consignments.manage','Create, allocate, route, and cancel consignments'),
    ('consignments.outbound','Confirm consignment stock-out')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
