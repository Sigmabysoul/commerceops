CREATE TABLE inventory_balances (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    product_id uuid NOT NULL,
    on_hand bigint NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
    reserved bigint NOT NULL DEFAULT 0 CHECK (reserved >= 0 AND reserved <= on_hand),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id,product_id),
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT
);

CREATE TABLE inventory_transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    product_id uuid NOT NULL,
    transaction_type text NOT NULL CHECK (transaction_type IN ('stock_in','manual_adjustment','correction')),
    quantity_delta bigint NOT NULL CHECK (quantity_delta <> 0),
    previous_balance bigint NOT NULL CHECK (previous_balance >= 0),
    resulting_balance bigint NOT NULL CHECK (resulting_balance >= 0),
    reason text NOT NULL CHECK (reason=btrim(reason) AND reason<>'' AND length(reason)<=500),
    reference_type text,
    reference_id text,
    actor_user_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,actor_user_id) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    CHECK ((reference_type IS NULL)=(reference_id IS NULL)),
    UNIQUE (company_id,id),
    UNIQUE (company_id,idempotency_key)
);
CREATE INDEX inventory_transactions_company_product_created_idx ON inventory_transactions(company_id,product_id,created_at DESC,id DESC);
CREATE INDEX inventory_transactions_company_type_created_idx ON inventory_transactions(company_id,transaction_type,created_at DESC,id DESC);

CREATE FUNCTION protect_inventory_transactions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'inventory transaction ledger is immutable' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER inventory_transactions_immutable
    BEFORE UPDATE OR DELETE ON inventory_transactions
    FOR EACH ROW EXECUTE FUNCTION protect_inventory_transactions();

INSERT INTO permissions(key,description) VALUES
    ('inventory.view','View inventory balances and transaction history'),
    ('inventory.stock_in','Record inventory stock-in transactions'),
    ('inventory.adjust','Record inventory adjustments and corrections')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
