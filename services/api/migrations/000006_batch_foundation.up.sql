CREATE TABLE batches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces(key) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','ready','cancelled')),
    created_by uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key = btrim(idempotency_key) AND idempotency_key <> '' AND length(idempotency_key) <= 128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    ready_at timestamptz,
    cancelled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, created_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, id),
    UNIQUE (company_id, idempotency_key)
);

CREATE INDEX batches_company_created_idx ON batches(company_id, created_at DESC, id DESC);

CREATE TABLE batch_members (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    batch_id uuid NOT NULL,
    marketplace_order_id uuid NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (batch_id, marketplace_order_id),
    FOREIGN KEY (company_id, batch_id) REFERENCES batches(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, marketplace_order_id) REFERENCES marketplace_orders(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, marketplace_order_id),
    UNIQUE (batch_id, position)
);

CREATE INDEX batch_members_company_batch_idx ON batch_members(company_id, batch_id, position);

INSERT INTO permissions(key, description) VALUES
 ('labels.print', 'Generate printable label output'),
 ('labels.reprint', 'Regenerate previously prepared label output')
ON CONFLICT (key) DO UPDATE SET description=EXCLUDED.description;
