ALTER TABLE marketplace_order_items
    ADD CONSTRAINT marketplace_order_items_company_id_id_key UNIQUE (company_id, id);

CREATE TABLE cancellations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_order_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','closed')),
    outbound_state text NOT NULL CHECK (outbound_state IN ('not_outbound','outbound_confirmed')),
    reason text NOT NULL CHECK (reason=btrim(reason) AND reason<>'' AND length(reason)<=500),
    cancelled_at timestamptz NOT NULL,
    recorded_by uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, marketplace_order_id) REFERENCES marketplace_orders(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, recorded_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (company_id, marketplace_order_id),
    UNIQUE (company_id, idempotency_key)
);
CREATE INDEX cancellations_company_status_created_idx
    ON cancellations(company_id, status, created_at DESC, id DESC);

CREATE TABLE return_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_order_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'expected' CHECK (status IN ('expected','received','inspection_pending','restocked','damaged','rejected','closed')),
    reason text NOT NULL CHECK (reason=btrim(reason) AND reason<>'' AND length(reason)<=500),
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    created_by uuid NOT NULL,
    received_by uuid,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    received_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, marketplace_order_id) REFERENCES marketplace_orders(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, created_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, received_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (company_id, idempotency_key),
    CHECK ((received_by IS NULL)=(received_at IS NULL))
);
CREATE INDEX return_cases_company_status_created_idx
    ON return_cases(company_id, status, created_at DESC, id DESC);
CREATE INDEX return_cases_company_order_created_idx
    ON return_cases(company_id, marketplace_order_id, created_at DESC);

CREATE TABLE return_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    return_case_id uuid NOT NULL,
    marketplace_order_item_id uuid NOT NULL,
    product_id uuid NOT NULL,
    expected_quantity integer NOT NULL CHECK (expected_quantity > 0),
    received_quantity integer CHECK (received_quantity IS NULL OR (received_quantity >= 0 AND received_quantity <= expected_quantity)),
    disposition text NOT NULL DEFAULT 'pending' CHECK (disposition IN ('pending','restockable','damaged','wrong_product','missing','rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, return_case_id) REFERENCES return_cases(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, marketplace_order_item_id) REFERENCES marketplace_order_items(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, product_id) REFERENCES products(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (return_case_id, marketplace_order_item_id)
);
CREATE INDEX return_items_company_product_idx
    ON return_items(company_id, product_id, created_at DESC);

CREATE TABLE return_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    return_case_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('created','received')),
    actor_user_id uuid NOT NULL,
    notes text CHECK (notes IS NULL OR (notes=btrim(notes) AND notes<>'' AND length(notes)<=2000)),
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, return_case_id) REFERENCES return_cases(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, actor_user_id) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (company_id, idempotency_key)
);
CREATE INDEX return_events_company_case_created_idx
    ON return_events(company_id, return_case_id, created_at, id);

CREATE FUNCTION protect_return_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'return event history is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER return_events_immutable
    BEFORE UPDATE OR DELETE ON return_events
    FOR EACH ROW EXECUTE FUNCTION protect_return_events();

INSERT INTO permissions(key, description) VALUES
    ('returns.view', 'View return and cancellation records'),
    ('returns.manage', 'Create and process return and cancellation records'),
    ('returns.restock', 'Accept inspected returns into sellable inventory')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
