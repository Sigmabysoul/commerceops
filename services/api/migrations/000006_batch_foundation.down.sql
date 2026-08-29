DELETE FROM role_permissions WHERE permission_key IN ('labels.print','labels.reprint');
DELETE FROM permissions WHERE key IN ('labels.print','labels.reprint');
DROP TABLE IF EXISTS batch_members;
DROP TABLE IF EXISTS batches;
