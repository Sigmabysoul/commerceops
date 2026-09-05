DROP INDEX IF EXISTS inventory_transactions_return_restock_source_idx;

ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction','ecommerce_out'));

ALTER TABLE return_events DROP CONSTRAINT return_events_event_type_check;
ALTER TABLE return_events ADD CONSTRAINT return_events_event_type_check
    CHECK (event_type IN ('created','received'));

ALTER TABLE return_items DROP CONSTRAINT return_items_corrected_restocked_check;
ALTER TABLE return_items DROP CONSTRAINT return_items_restocked_received_check;
ALTER TABLE return_items DROP COLUMN corrected_quantity;
ALTER TABLE return_items DROP COLUMN restocked_quantity;

ALTER TABLE return_cases DROP CONSTRAINT return_cases_status_check;
ALTER TABLE return_cases ADD CONSTRAINT return_cases_status_check
    CHECK (status IN ('expected','received','inspection_pending','restocked','damaged','rejected','closed'));
