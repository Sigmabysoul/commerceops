DELETE FROM permissions WHERE key IN ('marketplace_accounts.view', 'marketplace_accounts.manage');
DROP TABLE marketplace_accounts;
DROP TABLE business_identities;
