DROP TRIGGER IF EXISTS consignment_trace_evidence_immutable ON consignment_trace_evidence;
DROP TRIGGER IF EXISTS consignment_trace_box_allocations_immutable ON consignment_trace_box_allocations;
DROP TABLE IF EXISTS consignment_trace_evidence;
DROP TABLE IF EXISTS consignment_trace_box_allocations;
ALTER TABLE consignment_lines DROP CONSTRAINT IF EXISTS consignment_lines_trace_target_unique;

DROP TRIGGER IF EXISTS consignment_events_immutable ON consignment_events;
DELETE FROM consignment_events WHERE event_type IN ('trace_box_linked','trace_box_unlinked','trace_evidence_recorded');
ALTER TABLE consignment_events DROP CONSTRAINT consignment_events_event_type_check;
ALTER TABLE consignment_events ADD CONSTRAINT consignment_events_event_type_check CHECK (
    event_type IN ('created','allocated','status_changed','line_progress','outbound','completed','cancelled')
);
CREATE TRIGGER consignment_events_immutable BEFORE UPDATE OR DELETE ON consignment_events
    FOR EACH ROW EXECUTE FUNCTION protect_consignment_events();

ALTER TABLE consignments DROP COLUMN traceability_required;
