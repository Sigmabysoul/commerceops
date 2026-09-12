ALTER TABLE consignments ADD COLUMN traceability_required boolean NOT NULL DEFAULT false;

ALTER TABLE consignment_events DROP CONSTRAINT consignment_events_event_type_check;
ALTER TABLE consignment_events ADD CONSTRAINT consignment_events_event_type_check CHECK (event_type IN (
    'created','allocated','status_changed','line_progress','outbound','completed','cancelled',
    'trace_box_linked','trace_box_unlinked','trace_evidence_recorded'
));

ALTER TABLE consignment_lines ADD CONSTRAINT consignment_lines_trace_target_unique
    UNIQUE (company_id,id,consignment_id,product_id);

CREATE TABLE consignment_trace_box_allocations (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('trace_box_linked','trace_box_unlinked')),
    consignment_id uuid NOT NULL,
    consignment_line_id uuid NOT NULL,
    product_id uuid NOT NULL,
    trace_box_id uuid NOT NULL,
    quantity_delta bigint NOT NULL,
    source_allocation_event_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,event_id) REFERENCES consignment_events(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,consignment_line_id,consignment_id,product_id)
        REFERENCES consignment_lines(company_id,id,consignment_id,product_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,source_allocation_event_id)
        REFERENCES consignment_trace_box_allocations(company_id,event_id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id),
    CHECK ((event_type='trace_box_linked' AND quantity_delta>0 AND source_allocation_event_id IS NULL) OR
           (event_type='trace_box_unlinked' AND quantity_delta<0 AND source_allocation_event_id IS NOT NULL)),
    UNIQUE (company_id,source_allocation_event_id)
);
CREATE INDEX consignment_trace_allocations_line_idx
    ON consignment_trace_box_allocations(company_id,consignment_line_id,created_at,event_id);
CREATE INDEX consignment_trace_allocations_box_product_idx
    ON consignment_trace_box_allocations(company_id,trace_box_id,product_id);

CREATE TABLE consignment_trace_evidence (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    consignment_id uuid NOT NULL,
    evidence_type text NOT NULL CHECK (evidence_type IN ('pouch_reference','file_reference')),
    reference_value text NOT NULL CHECK (
        reference_value=btrim(reference_value) AND reference_value<>'' AND length(reference_value)<=500
    ),
    trace_box_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,event_id) REFERENCES consignment_events(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,consignment_id) REFERENCES consignments(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id)
);
CREATE INDEX consignment_trace_evidence_reference_idx
    ON consignment_trace_evidence(company_id,evidence_type,reference_value);
CREATE INDEX consignment_trace_evidence_consignment_idx
    ON consignment_trace_evidence(company_id,consignment_id,created_at,event_id);

CREATE TRIGGER consignment_trace_box_allocations_immutable
    BEFORE UPDATE OR DELETE ON consignment_trace_box_allocations
    FOR EACH ROW EXECUTE FUNCTION protect_consignment_events();
CREATE TRIGGER consignment_trace_evidence_immutable
    BEFORE UPDATE OR DELETE ON consignment_trace_evidence
    FOR EACH ROW EXECUTE FUNCTION protect_consignment_events();
