DELETE FROM role_permissions
WHERE permission_key IN ('returns.view','returns.manage','returns.restock');
DELETE FROM permissions
WHERE key IN ('returns.view','returns.manage','returns.restock');

DROP TABLE IF EXISTS return_events;
DROP FUNCTION IF EXISTS protect_return_events();
DROP TABLE IF EXISTS return_items;
DROP TABLE IF EXISTS return_cases;
DROP TABLE IF EXISTS cancellations;

ALTER TABLE marketplace_order_items
    DROP CONSTRAINT IF EXISTS marketplace_order_items_company_id_id_key;
