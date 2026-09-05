-- Device identity and credentials are separate so credentials can be rotated or
-- revoked without changing the tenant-owned agent record.
CREATE TABLE printer_agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    friendly_name text NOT NULL CHECK (friendly_name=btrim(friendly_name) AND friendly_name<>'' AND length(friendly_name)<=120),
    platform text NOT NULL CHECK (platform IN ('linux_cups','windows')),
    status text NOT NULL DEFAULT 'offline' CHECK (status IN ('online','offline','revoked')),
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities)='object'),
    last_seen_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(company_id,id),
    UNIQUE(company_id,friendly_name)
);

CREATE TABLE printer_agent_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(company_id,agent_id) REFERENCES printer_agents(company_id,id) ON DELETE CASCADE
);
CREATE INDEX printer_agent_credentials_agent_idx ON printer_agent_credentials(company_id,agent_id) WHERE revoked_at IS NULL;

-- Only the agent reports os_printer_id. Browser print requests reference id.
CREATE TABLE registered_printers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    friendly_name text NOT NULL CHECK (friendly_name=btrim(friendly_name) AND friendly_name<>'' AND length(friendly_name)<=120),
    os_printer_id text NOT NULL CHECK (os_printer_id=btrim(os_printer_id) AND os_printer_id<>'' AND length(os_printer_id)<=255),
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities)='object'),
    location text CHECK (location IS NULL OR (location=btrim(location) AND location<>'' AND length(location)<=255)),
    status text NOT NULL DEFAULT 'offline' CHECK (status IN ('online','offline')),
    enabled boolean NOT NULL DEFAULT true,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(company_id,agent_id) REFERENCES printer_agents(company_id,id) ON DELETE RESTRICT,
    UNIQUE(company_id,id),
    UNIQUE(company_id,friendly_name),
    UNIQUE(company_id,agent_id,os_printer_id)
);
CREATE INDEX registered_printers_agent_idx ON registered_printers(company_id,agent_id,status,enabled);

-- PDF bytes live in object storage; this table owns validated, reusable metadata.
CREATE TABLE print_library_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (name=btrim(name) AND name<>'' AND length(name)<=120),
    category text NOT NULL CHECK (category=btrim(category) AND category<>'' AND length(category)<=80),
    description text CHECK (description IS NULL OR length(description)<=1000),
    storage_key text NOT NULL,
    content_type text NOT NULL DEFAULT 'application/pdf' CHECK (content_type='application/pdf'),
    size_bytes bigint NOT NULL CHECK (size_bytes>0 AND size_bytes<=20971520),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    page_count integer NOT NULL CHECK (page_count>0 AND page_count<=500),
    default_printer_id uuid,
    default_copies integer NOT NULL DEFAULT 1 CHECK (default_copies BETWEEN 1 AND 100),
    product_id uuid,
    favorite boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    uploaded_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(company_id,default_printer_id) REFERENCES registered_printers(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,uploaded_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE(company_id,id),
    UNIQUE(company_id,storage_key)
);
CREATE INDEX print_library_assets_browse_idx ON print_library_assets(company_id,active,favorite DESC,category,name);

-- Existing print_jobs generate PDFs. printer_jobs are physical delivery attempts.
CREATE TABLE printer_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    requested_by uuid NOT NULL,
    printer_id uuid NOT NULL,
    print_artifact_id uuid,
    print_library_asset_id uuid,
    copies integer NOT NULL CHECK (copies BETWEEN 1 AND 100),
    origin_type text NOT NULL CHECK (origin_type IN ('ecommerce_batch','ecommerce_reprint','consignment','quick_print')),
    origin_reference text NOT NULL CHECK (origin_reference=btrim(origin_reference) AND origin_reference<>'' AND length(origin_reference)<=255),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','claimed','printing','completed','failed','cancelled')),
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    lease_agent_id uuid,
    lease_token_hash bytea CHECK (lease_token_hash IS NULL OR octet_length(lease_token_hash)=32),
    lease_expires_at timestamptz,
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count>=0 AND attempt_count<=20),
    source_printer_job_id uuid,
    failure_code text,
    failure_message text CHECK (failure_message IS NULL OR length(failure_message)<=1000),
    claimed_at timestamptz,
    printing_at timestamptz,
    completed_at timestamptz,
    failed_at timestamptz,
    cancelled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(company_id,requested_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,printer_id) REFERENCES registered_printers(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,print_artifact_id) REFERENCES print_artifacts(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,print_library_asset_id) REFERENCES print_library_assets(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,lease_agent_id) REFERENCES printer_agents(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,source_printer_job_id) REFERENCES printer_jobs(company_id,id) ON DELETE RESTRICT,
    CHECK ((print_artifact_id IS NULL)<>(print_library_asset_id IS NULL)),
    CHECK ((lease_agent_id IS NULL)=(lease_token_hash IS NULL) AND (lease_agent_id IS NULL)=(lease_expires_at IS NULL)),
    UNIQUE(company_id,id),
    UNIQUE(company_id,idempotency_key)
);
CREATE INDEX printer_jobs_claim_idx ON printer_jobs(company_id,printer_id,created_at) WHERE status='queued';
CREATE INDEX printer_jobs_history_idx ON printer_jobs(company_id,created_at DESC);

-- Lifecycle events are append-only evidence and never drive Inventory.
CREATE TABLE printer_job_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    printer_job_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('queued','claimed','printing','completed','failed','cancelled','lease_expired','retried')),
    actor_user_id uuid,
    actor_agent_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata)='object'),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(company_id,printer_job_id) REFERENCES printer_jobs(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,actor_user_id) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    FOREIGN KEY(company_id,actor_agent_id) REFERENCES printer_agents(company_id,id) ON DELETE RESTRICT,
    CHECK (NOT (actor_user_id IS NOT NULL AND actor_agent_id IS NOT NULL))
);
CREATE INDEX printer_job_events_job_idx ON printer_job_events(company_id,printer_job_id,occurred_at,id);

INSERT INTO permissions(key,description) VALUES
    ('printers.view','View registered printers and agents'),
    ('printers.manage','Register and manage printers and agents'),
    ('printing.print','Request and view physical print jobs'),
    ('printing.reprint','Retry or reprint physical print jobs'),
    ('print_library.view','View reusable print assets'),
    ('print_library.manage','Upload and manage reusable print assets')
ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description;
