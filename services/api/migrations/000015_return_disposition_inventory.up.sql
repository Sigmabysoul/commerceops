ALTER TABLE return_cases DROP CONSTRAINT return_cases_status_check;
ALTER TABLE return_cases ADD CONSTRAINT return_cases_status_check
    CHECK (status IN ('expected','received','inspected','inspection_pending','restocked','restock_corrected','damaged','rejected','closed'));

ALTER TABLE return_items
    ADD COLUMN restocked_quantity integer NOT NULL DEFAULT 0 CHECK (restocked_quantity >= 0),
    ADD COLUMN corrected_quantity integer NOT NULL DEFAULT 0 CHECK (corrected_quantity >= 0),
    ADD CONSTRAINT return_items_restocked_received_check
        CHECK (restocked_quantity <= COALESCE(received_quantity,0)),
    ADD CONSTRAINT return_items_corrected_restocked_check
        CHECK (corrected_quantity <= restocked_quantity);

ALTER TABLE return_events DROP CONSTRAINT return_events_event_type_check;
ALTER TABLE return_events ADD CONSTRAINT return_events_event_type_check
    CHECK (event_type IN ('created','received','inspected','restocked','restock_corrected'));

ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction','ecommerce_out','return_restock'));

CREATE UNIQUE INDEX inventory_transactions_return_restock_source_idx
    ON inventory_transactions(company_id,reference_type,reference_id,product_id)
    WHERE transaction_type='return_restock';
