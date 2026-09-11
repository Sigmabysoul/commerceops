ALTER TABLE trace_box_events DROP CONSTRAINT trace_box_events_event_type_check;
ALTER TABLE trace_box_events ADD CONSTRAINT trace_box_events_event_type_check CHECK (event_type IN (
    'box_created','content_added','content_removed','custody_transferred',
    'qc_completed','work_completed','handover_sent','handover_received',
    'packing_completed','final_check_completed','ready_for_shipment'
));

ALTER TABLE trace_box_custody_changes DROP CONSTRAINT trace_box_custody_changes_event_type_check;
ALTER TABLE trace_box_custody_changes ADD CONSTRAINT trace_box_custody_changes_event_type_check
    CHECK (event_type IN ('custody_transferred','handover_received'));

CREATE TABLE trace_box_qc_checks (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL DEFAULT 'qc_completed' CHECK (event_type='qc_completed'),
    trace_box_id uuid NOT NULL,
    checked_by_employee_id uuid NOT NULL,
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id)
        REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,checked_by_employee_id)
        REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id)
);

CREATE TABLE trace_box_qc_lines (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    qc_event_id uuid NOT NULL,
    trace_box_id uuid NOT NULL,
    product_id uuid NOT NULL,
    checked_quantity bigint NOT NULL CHECK (checked_quantity>0),
    passed_quantity bigint NOT NULL CHECK (passed_quantity>=0),
    rejected_quantity bigint NOT NULL CHECK (rejected_quantity>=0),
    rejection_reason text CHECK (rejection_reason IN (
        'damaged','wrong_sticker','dirty','missing_component','packaging_damaged',
        'wrong_product','manufacturing_defect','other'
    )),
    required_work text CHECK (required_work IN (
        'sticker_replacement','cleaning','repacking','component_check','repair','other'
    )),
    CHECK (checked_quantity=passed_quantity+rejected_quantity),
    CHECK ((rejected_quantity=0 AND rejection_reason IS NULL AND required_work IS NULL) OR
           (rejected_quantity>0 AND rejection_reason IS NOT NULL AND required_work IS NOT NULL)),
    FOREIGN KEY (company_id,qc_event_id) REFERENCES trace_box_qc_checks(company_id,event_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,product_id) REFERENCES products(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,qc_event_id,product_id)
);
CREATE INDEX trace_box_qc_lines_box_idx ON trace_box_qc_lines(company_id,trace_box_id,product_id);

CREATE TABLE trace_box_work_requirements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    trace_box_id uuid NOT NULL,
    qc_event_id uuid NOT NULL,
    product_id uuid NOT NULL,
    quantity bigint NOT NULL CHECK (quantity>0),
    work_type text NOT NULL CHECK (work_type IN (
        'sticker_replacement','cleaning','repacking','component_check','repair','other'
    )),
    rejection_reason text NOT NULL CHECK (rejection_reason IN (
        'damaged','wrong_sticker','dirty','missing_component','packaging_damaged',
        'wrong_product','manufacturing_defect','other'
    )),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,qc_event_id,product_id)
        REFERENCES trace_box_qc_lines(company_id,qc_event_id,product_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,trace_box_id) REFERENCES trace_boxes(company_id,id) ON DELETE RESTRICT,
    UNIQUE (company_id,id),
    UNIQUE (company_id,qc_event_id,product_id)
);
CREATE INDEX trace_box_work_requirements_box_idx ON trace_box_work_requirements(company_id,trace_box_id,created_at,id);

CREATE TABLE trace_box_work_completions (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL DEFAULT 'work_completed' CHECK (event_type='work_completed'),
    trace_box_id uuid NOT NULL,
    work_requirement_id uuid NOT NULL,
    completed_by_employee_id uuid NOT NULL,
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id)
        REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,work_requirement_id)
        REFERENCES trace_box_work_requirements(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,completed_by_employee_id)
        REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id),
    UNIQUE (company_id,work_requirement_id)
);

CREATE TABLE trace_box_handovers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    trace_box_id uuid NOT NULL,
    sent_event_id uuid NOT NULL,
    sent_event_type text NOT NULL DEFAULT 'handover_sent' CHECK (sent_event_type='handover_sent'),
    target_employee_id uuid,
    target_department_id uuid,
    sent_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id,sent_event_id,sent_event_type,trace_box_id)
        REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,target_employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,target_department_id) REFERENCES consignment_departments(company_id,id) ON DELETE RESTRICT,
    CHECK ((target_employee_id IS NOT NULL)::integer + (target_department_id IS NOT NULL)::integer = 1),
    UNIQUE (company_id,id),
    UNIQUE (company_id,sent_event_id)
);
CREATE INDEX trace_box_handovers_box_idx ON trace_box_handovers(company_id,trace_box_id,sent_at,id);

CREATE TABLE trace_box_handover_receipts (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL DEFAULT 'handover_received' CHECK (event_type='handover_received'),
    trace_box_id uuid NOT NULL,
    handover_id uuid NOT NULL,
    received_by_employee_id uuid NOT NULL,
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id)
        REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,handover_id) REFERENCES trace_box_handovers(company_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,received_by_employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id,event_id),
    UNIQUE (company_id,handover_id)
);

CREATE TABLE trace_box_gate_records (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    event_id uuid NOT NULL,
    event_type text NOT NULL CHECK (event_type IN ('packing_completed','final_check_completed','ready_for_shipment')),
    trace_box_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    passed boolean,
    FOREIGN KEY (company_id,event_id,event_type,trace_box_id)
        REFERENCES trace_box_events(company_id,id,event_type,trace_box_id) ON DELETE RESTRICT,
    FOREIGN KEY (company_id,employee_id) REFERENCES employees(company_id,id) ON DELETE RESTRICT,
    CHECK ((event_type='final_check_completed' AND passed IS NOT NULL) OR
           (event_type<>'final_check_completed' AND passed IS NULL)),
    PRIMARY KEY (company_id,event_id)
);

CREATE TRIGGER trace_box_qc_checks_immutable BEFORE UPDATE OR DELETE ON trace_box_qc_checks
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_qc_lines_immutable BEFORE UPDATE OR DELETE ON trace_box_qc_lines
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_work_requirements_immutable BEFORE UPDATE OR DELETE ON trace_box_work_requirements
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_work_completions_immutable BEFORE UPDATE OR DELETE ON trace_box_work_completions
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_handovers_immutable BEFORE UPDATE OR DELETE ON trace_box_handovers
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_handover_receipts_immutable BEFORE UPDATE OR DELETE ON trace_box_handover_receipts
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
CREATE TRIGGER trace_box_gate_records_immutable BEFORE UPDATE OR DELETE ON trace_box_gate_records
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
