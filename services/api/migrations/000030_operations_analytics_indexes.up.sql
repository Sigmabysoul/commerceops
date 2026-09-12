-- Reporting filters immutable operational events by company and completion instant.
CREATE INDEX trace_box_events_company_time_idx ON trace_box_events(company_id,created_at,event_type,id);
