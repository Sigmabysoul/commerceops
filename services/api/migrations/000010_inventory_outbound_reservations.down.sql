DELETE FROM role_permissions WHERE permission_key='inventory.dispatch';
DELETE FROM permissions WHERE key='inventory.dispatch';
DROP TABLE IF EXISTS inventory_reservations;
DROP TABLE IF EXISTS inventory_outbound_events;
DROP INDEX IF EXISTS inventory_transactions_ecommerce_source_idx;
ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction'));
