ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction','ecommerce_out'));

CREATE UNIQUE INDEX inventory_transactions_ecommerce_source_idx
    ON inventory_transactions(company_id,reference_type,reference_id,product_id)
    WHERE transaction_type='ecommerce_out';

CREATE TABLE inventory_outbound_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    batch_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,batch_id) REFERENCES batches(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,actor_user_id) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,batch_id),
    UNIQUE (company_id,idempotency_key)
);

CREATE TABLE inventory_reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    product_id uuid NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','released')),
    reason text NOT NULL CHECK (reason=btrim(reason) AND reason<>'' AND length(reason)<=500),
    source_type text NOT NULL CHECK (source_type=btrim(source_type) AND source_type<>'' AND length(source_type)<=100),
    source_id text NOT NULL CHECK (source_id=btrim(source_id) AND source_id<>'' AND length(source_id)<=200),
    created_by uuid NOT NULL,
    released_by uuid,
    release_reason text CHECK (release_reason IS NULL OR (release_reason=btrim(release_reason) AND release_reason<>'' AND length(release_reason)<=500)),
    create_idempotency_key text NOT NULL CHECK (create_idempotency_key=btrim(create_idempotency_key) AND create_idempotency_key<>'' AND length(create_idempotency_key)<=128),
    create_request_hash text NOT NULL CHECK (create_request_hash ~ '^[0-9a-f]{64}$'),
    release_idempotency_key text,
    release_request_hash text,
    created_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,created_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,released_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    CHECK ((status='active' AND released_by IS NULL AND release_reason IS NULL AND release_idempotency_key IS NULL AND release_request_hash IS NULL AND released_at IS NULL)
        OR (status='released' AND released_by IS NOT NULL AND release_reason IS NOT NULL AND release_idempotency_key IS NOT NULL AND release_request_hash ~ '^[0-9a-f]{64}$' AND released_at IS NOT NULL)),
    UNIQUE (company_id,id),
    UNIQUE (company_id,create_idempotency_key),
    UNIQUE (company_id,source_type,source_id,product_id)
);
CREATE INDEX inventory_reservations_company_status_created_idx ON inventory_reservations(company_id,status,created_at DESC,id DESC);
CREATE INDEX inventory_reservations_company_product_status_idx ON inventory_reservations(company_id,product_id,status);
CREATE UNIQUE INDEX inventory_reservations_company_release_key_idx ON inventory_reservations(company_id,release_idempotency_key) WHERE release_idempotency_key IS NOT NULL;

INSERT INTO permissions(key,description) VALUES
    ('inventory.dispatch','Confirm ecommerce batch stock-out')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
