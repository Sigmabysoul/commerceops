DELETE FROM role_permissions WHERE permission_key IN ('inventory.view','inventory.stock_in','inventory.adjust');
DELETE FROM permissions WHERE key IN ('inventory.view','inventory.stock_in','inventory.adjust');
DROP TRIGGER IF EXISTS inventory_transactions_immutable ON inventory_transactions;
DROP FUNCTION IF EXISTS protect_inventory_transactions();
DROP TABLE IF EXISTS inventory_transactions;
DROP TABLE IF EXISTS inventory_balances;
