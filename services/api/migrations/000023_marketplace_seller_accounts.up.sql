-- Phase 16 keeps the company boundary intact while recording business/trading
-- identities separately from printer agents and workstations.
CREATE TABLE business_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    display_name text NOT NULL CHECK (display_name = btrim(display_name) AND length(display_name) BETWEEN 1 AND 120),
    legal_name text CHECK (legal_name IS NULL OR (legal_name = btrim(legal_name) AND length(legal_name) BETWEEN 1 AND 250)),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'dormant', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (company_id, id),
    UNIQUE (company_id, display_name)
);

CREATE TABLE marketplace_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces(key),
    business_identity_id uuid NOT NULL,
    internal_key text NOT NULL CHECK (internal_key ~ '^[a-z][a-z0-9_]{0,79}$'),
    display_name text NOT NULL CHECK (display_name = btrim(display_name) AND length(display_name) BETWEEN 1 AND 120),
    external_seller_id text CHECK (external_seller_id IS NULL OR (external_seller_id = btrim(external_seller_id) AND length(external_seller_id) BETWEEN 1 AND 200)),
    integration_mode text NOT NULL CHECK (integration_mode IN ('manual_upload', 'api', 'file_watch', 'print_capture', 'partner_connector')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'dormant', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, business_identity_id) REFERENCES business_identities(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, id, marketplace_key),
    UNIQUE (company_id, internal_key)
);

CREATE INDEX marketplace_accounts_company_marketplace_status_idx
    ON marketplace_accounts(company_id, marketplace_key, status);

INSERT INTO permissions(key, description) VALUES
    ('marketplace_accounts.view', 'View trading identities and marketplace seller accounts'),
    ('marketplace_accounts.manage', 'Manage trading identities and marketplace seller accounts');
