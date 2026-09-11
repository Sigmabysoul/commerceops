DROP TRIGGER IF EXISTS trace_box_gate_records_immutable ON trace_box_gate_records;
DROP TRIGGER IF EXISTS trace_box_handover_receipts_immutable ON trace_box_handover_receipts;
DROP TRIGGER IF EXISTS trace_box_handovers_immutable ON trace_box_handovers;
DROP TRIGGER IF EXISTS trace_box_work_completions_immutable ON trace_box_work_completions;
DROP TRIGGER IF EXISTS trace_box_work_requirements_immutable ON trace_box_work_requirements;
DROP TRIGGER IF EXISTS trace_box_qc_lines_immutable ON trace_box_qc_lines;
DROP TRIGGER IF EXISTS trace_box_qc_checks_immutable ON trace_box_qc_checks;
DROP TABLE IF EXISTS trace_box_gate_records;
DROP TABLE IF EXISTS trace_box_handover_receipts;
DROP TABLE IF EXISTS trace_box_handovers;
DROP TABLE IF EXISTS trace_box_work_completions;
DROP TABLE IF EXISTS trace_box_work_requirements;
DROP TABLE IF EXISTS trace_box_qc_lines;
DROP TABLE IF EXISTS trace_box_qc_checks;

DROP TRIGGER IF EXISTS trace_box_custody_changes_immutable ON trace_box_custody_changes;
DELETE FROM trace_box_custody_changes WHERE event_type='handover_received';
ALTER TABLE trace_box_custody_changes DROP CONSTRAINT trace_box_custody_changes_event_type_check;
ALTER TABLE trace_box_custody_changes ADD CONSTRAINT trace_box_custody_changes_event_type_check
    CHECK (event_type='custody_transferred');
ALTER TABLE trace_box_custody_changes ALTER COLUMN event_type SET DEFAULT 'custody_transferred';
CREATE TRIGGER trace_box_custody_changes_immutable BEFORE UPDATE OR DELETE ON trace_box_custody_changes
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();

DROP TRIGGER IF EXISTS trace_box_events_immutable ON trace_box_events;
DELETE FROM trace_box_events WHERE event_type IN (
    'qc_completed','work_completed','handover_sent','handover_received',
    'packing_completed','final_check_completed','ready_for_shipment'
);
ALTER TABLE trace_box_events DROP CONSTRAINT trace_box_events_event_type_check;
ALTER TABLE trace_box_events ADD CONSTRAINT trace_box_events_event_type_check CHECK (
    event_type IN ('box_created','content_added','content_removed','custody_transferred')
);
CREATE TRIGGER trace_box_events_immutable BEFORE UPDATE OR DELETE ON trace_box_events
    FOR EACH ROW EXECUTE FUNCTION protect_traceability_history();
