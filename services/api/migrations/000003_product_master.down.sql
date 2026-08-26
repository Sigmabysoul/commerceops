DROP TABLE IF EXISTS sku_mappings;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS marketplaces;

DELETE FROM role_permissions WHERE permission_key IN ('products.view', 'products.manage');
DELETE FROM permissions WHERE key IN ('products.view', 'products.manage');
