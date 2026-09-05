DELETE FROM role_permissions WHERE permission_key IN ('consignments.view','consignments.work','consignments.manage','consignments.outbound');
DELETE FROM permissions WHERE key IN ('consignments.view','consignments.work','consignments.manage','consignments.outbound');
DROP INDEX IF EXISTS inventory_transactions_consignment_source_idx;
DROP TRIGGER IF EXISTS consignment_events_immutable ON consignment_events;
DROP FUNCTION IF EXISTS protect_consignment_events();
DROP TABLE IF EXISTS consignment_events;
DROP TABLE IF EXISTS consignment_lines;
DROP TABLE IF EXISTS consignments;
DROP TABLE IF EXISTS consignment_department_members;
DROP TABLE IF EXISTS consignment_departments;
ALTER TABLE inventory_transactions DROP CONSTRAINT inventory_transactions_transaction_type_check;
ALTER TABLE inventory_transactions ADD CONSTRAINT inventory_transactions_transaction_type_check
    CHECK (transaction_type IN ('stock_in','manual_adjustment','correction','ecommerce_out','return_restock'));
