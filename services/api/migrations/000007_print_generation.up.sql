CREATE TABLE print_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    batch_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    status text NOT NULL DEFAULT 'generating' CHECK (status IN ('generating','ready','failed')),
    sort_labels boolean NOT NULL,
    export_invoices boolean NOT NULL,
    generation_version text NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key=btrim(idempotency_key) AND idempotency_key<>'' AND length(idempotency_key)<=128),
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    error_code text,
    error_message text,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,batch_id) REFERENCES batches(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,requested_by) REFERENCES company_users(company_id,user_id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,idempotency_key)
);
CREATE INDEX print_jobs_company_batch_idx ON print_jobs(company_id,batch_id,created_at DESC);

CREATE TABLE print_job_items (
    company_id uuid NOT NULL,
    print_job_id uuid NOT NULL,
    marketplace_order_id uuid NOT NULL,
    source_file_id uuid NOT NULL,
    processing_job_id uuid NOT NULL,
    source_page integer NOT NULL CHECK (source_page>0),
    output_position integer NOT NULL CHECK (output_position>0),
    PRIMARY KEY (print_job_id,marketplace_order_id),
    FOREIGN KEY (company_id,print_job_id) REFERENCES print_jobs(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,marketplace_order_id) REFERENCES marketplace_orders(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,source_file_id) REFERENCES source_files(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,processing_job_id) REFERENCES processing_jobs(company_id,id) ON DELETE RESTRICT,
    UNIQUE (print_job_id,output_position)
);

CREATE TABLE print_artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    print_job_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('labels','invoices')),
    storage_key text NOT NULL,
    content_type text NOT NULL DEFAULT 'application/pdf',
    size_bytes bigint NOT NULL CHECK (size_bytes>0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    page_count integer NOT NULL CHECK (page_count>0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,print_job_id) REFERENCES print_jobs(company_id,id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (print_job_id,kind),
    UNIQUE (company_id,storage_key)
);
