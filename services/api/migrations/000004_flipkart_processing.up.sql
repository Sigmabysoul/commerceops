CREATE TABLE source_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces(key) ON DELETE RESTRICT,
    storage_key text NOT NULL,
    original_filename text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    uploaded_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, uploaded_by) REFERENCES company_users(company_id, user_id) ON DELETE RESTRICT,
    UNIQUE (company_id, marketplace_key, sha256),
    UNIQUE (company_id, id)
);

CREATE TABLE processing_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    source_file_id uuid NOT NULL,
    marketplace_key text NOT NULL REFERENCES marketplaces(key) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','processing','needs_review','processed','failed')),
    parser_version text NOT NULL,
    total_pages integer NOT NULL DEFAULT 0 CHECK (total_pages >= 0),
    processed_pages integer NOT NULL DEFAULT 0 CHECK (processed_pages >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, source_file_id) REFERENCES source_files(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, id)
);
CREATE INDEX processing_jobs_company_created_idx ON processing_jobs(company_id, created_at DESC);

CREATE TABLE marketplace_orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    marketplace_key text NOT NULL REFERENCES marketplaces(key) ON DELETE RESTRICT,
    source_file_id uuid NOT NULL,
    processing_job_id uuid NOT NULL,
    source_page integer NOT NULL CHECK (source_page > 0),
    marketplace_order_id text,
    awb text,
    status text NOT NULL CHECK (status IN ('resolved','needs_review','duplicate')),
    parser_version text NOT NULL,
    extraction_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, source_file_id) REFERENCES source_files(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, processing_job_id) REFERENCES processing_jobs(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, id)
);
CREATE UNIQUE INDEX marketplace_orders_awb_unique ON marketplace_orders(company_id, marketplace_key, awb) WHERE awb IS NOT NULL AND status <> 'duplicate';
CREATE UNIQUE INDEX marketplace_orders_order_id_unique ON marketplace_orders(company_id, marketplace_key, marketplace_order_id) WHERE marketplace_order_id IS NOT NULL AND status <> 'duplicate';

CREATE TABLE marketplace_order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL,
    raw_sku text,
    product_id uuid,
    quantity integer CHECK (quantity IS NULL OR quantity > 0),
    quantity_source text NOT NULL CHECK (quantity_source IN ('extracted','missing')),
    resolution_status text NOT NULL CHECK (resolution_status IN ('resolved','unresolved')),
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, order_id) REFERENCES marketplace_orders(company_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id, product_id) REFERENCES products(company_id, id) ON DELETE RESTRICT
);

CREATE TABLE processing_errors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    processing_job_id uuid NOT NULL,
    source_page integer,
    severity text NOT NULL CHECK (severity IN ('warning','error')),
    code text NOT NULL,
    message text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, processing_job_id) REFERENCES processing_jobs(company_id, id) ON DELETE CASCADE
);

INSERT INTO permissions(key, description) VALUES
 ('labels.upload', 'Upload marketplace label files'),
 ('labels.process', 'Process marketplace label files')
ON CONFLICT (key) DO UPDATE SET description=EXCLUDED.description;
