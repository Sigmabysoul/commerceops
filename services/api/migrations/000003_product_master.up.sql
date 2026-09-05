CREATE TABLE marketplaces (
    key text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*$'),
    display_name text NOT NULL CHECK (btrim(display_name) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO marketplaces (key, display_name) VALUES
    ('flipkart', 'Flipkart'),
    ('amazon', 'Amazon'),
    ('meesho', 'Meesho'),
    ('myntra', 'Myntra'),
    ('snapdeal', 'Snapdeal');

CREATE TABLE products (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    internal_code text NOT NULL CHECK (internal_code = btrim(internal_code) AND internal_code <> ''),
    name text NOT NULL CHECK (name = btrim(name) AND name <> ''),
    brand text,
    variant text,
    size text,
    pack_type text,
    unit_count integer CHECK (unit_count IS NULL OR unit_count > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (company_id, internal_code),
    UNIQUE (company_id, id)
);

CREATE INDEX products_company_status_name_idx ON products (company_id, status, name, id);

CREATE TABLE sku_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces (key) ON DELETE RESTRICT,
    product_id uuid NOT NULL,
    sku text NOT NULL CHECK (sku = btrim(sku) AND sku <> ''),
    quantity_multiplier integer NOT NULL DEFAULT 1 CHECK (quantity_multiplier > 0),
    interpretation_metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(interpretation_metadata) = 'object'),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sku_mappings_company_product_fk
        FOREIGN KEY (company_id, product_id)
        REFERENCES products (company_id, id)
        ON DELETE RESTRICT,
    UNIQUE (company_id, id)
);

CREATE UNIQUE INDEX sku_mappings_active_lookup_unique
    ON sku_mappings (company_id, marketplace_key, sku)
    WHERE status = 'active';

CREATE INDEX sku_mappings_company_product_idx ON sku_mappings (company_id, product_id, status);

INSERT INTO permissions (key, description) VALUES
    ('products.view', 'View products and SKU mappings'),
    ('products.manage', 'Create and update products and SKU mappings')
ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description;
