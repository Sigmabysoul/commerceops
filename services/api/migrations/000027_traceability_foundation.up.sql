CREATE TABLE trace_boxes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    opaque_identifier text NOT NULL UNIQUE
        CHECK (opaque_identifier ~ '^TBX_[A-Z2-7]{26}$'),
    label text CHECK (label IS NULL OR (label=btrim(label) AND label<>'' AND length(label)<=200)),
    created_by uuid NOT NULL,
    idempotency_key text NOT NULL
        CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,created_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,idempotency_key)
);
CREATE INDEX trace_boxes_company_created_idx ON trace_boxes(company_id,created_at DESC,id DESC);

CREATE TABLE trace_box_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    trace_box_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('box_created','content_added','content_removed','custody_transferred')),
    actor_user_id uuid NOT NULL,
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object'),
    idempotency_key text NOT NULL
        CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,actor_user_id) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,id,event_type,trace_box_id),
    UNIQUE (company_id,idempotency_key)
);
CREATE INDEX trace_box_events_company_box_idx ON trace_box_events(company_id,trace_box_id,created_at,id);

CREATE TABLE trace_box_content_changes (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('content_added','content_removed')),
    trace_box_id uuid NOT NULL,
    product_id uuid NOT NULL,
    quantity_delta bigint NOT NULL CHECK (
        (event_type='content_added' AND quantity_delta>0) OR
        (event_type='content_removed' AND quantity_delta<0)
    ),
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id) REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id)
);
CREATE INDEX trace_box_content_changes_current_idx ON trace_box_content_changes(company_id,trace_box_id,product_id);

CREATE TABLE trace_box_custody_changes (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL DEFAULT 'custody_transferred' CHECK (event_type='custody_transferred'),
    trace_box_id uuid NOT NULL,
    employee_id uuid,
    department_id uuid,
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id) REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,department_id) REFERENCES consignment_departments(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id),
    CHECK ((employee_id IS NOT NULL)::integer + (department_id IS NOT NULL)::integer = 1)
);
CREATE INDEX trace_box_custody_changes_current_idx ON trace_box_custody_changes(company_id,trace_box_id,event_id);

CREATE FUNCTION protect_traceability_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'traceability history is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER trace_box_events_immutable BEFORE UPDATE OR DELETE ON trace_box_events
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_content_changes_immutable BEFORE UPDATE OR DELETE ON trace_box_content_changes
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_custody_changes_immutable BEFORE UPDATE OR DELETE ON trace_box_custody_changes
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();

INSERT INTO permissions(key,description) VALUES
    ('traceability.view','View Trace Boxes, contents, custody and event history'),
    ('traceability.manage','Create Trace Boxes and record content or custody changes')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
