DROP TABLE IF EXISTS cancellation_events;
DROP FUNCTION IF EXISTS protect_cancellation_events();
DROP INDEX IF EXISTS cancellations_company_cancelled_at_idx;
DROP INDEX IF EXISTS return_events_company_type_created_idx;

ALTER TABLE return_events DROP CONSTRAINT return_events_event_type_check;
ALTER TABLE return_events ADD CONSTRAINT return_events_event_type_check
    CHECK (event_type IN ('created','received','inspected','restocked','restock_corrected'));

ALTER TABLE return_cases DROP CONSTRAINT return_cases_closed_state_check;
ALTER TABLE return_cases DROP CONSTRAINT return_cases_closed_by_fkey;
ALTER TABLE return_cases DROP COLUMN closed_at;
ALTER TABLE return_cases DROP COLUMN closed_by;

ALTER TABLE cancellations DROP CONSTRAINT cancellations_closed_state_check;
ALTER TABLE cancellations DROP CONSTRAINT cancellations_closed_by_fkey;
ALTER TABLE cancellations DROP COLUMN closed_at;
ALTER TABLE cancellations DROP COLUMN closed_by;
