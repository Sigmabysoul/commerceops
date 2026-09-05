CREATE TABLE marketplace_order_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL,
    source_file_id uuid NOT NULL,
    source_page integer NOT NULL CHECK (source_page > 0),
    document_role text NOT NULL CHECK (document_role IN ('shipping_label','invoice')),
    extraction_method text NOT NULL CHECK (extraction_method IN ('text','ocr')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id, order_id) REFERENCES marketplace_orders(company_id, id) ON DELETE CASCADE,
    FOREIGN KEY (company_id, source_file_id) REFERENCES source_files(company_id, id) ON DELETE RESTRICT,
    UNIQUE (company_id, order_id, source_page, document_role)
);

CREATE INDEX marketplace_order_documents_order_idx
    ON marketplace_order_documents(company_id, order_id, source_page);
