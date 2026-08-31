RULE INV-001

Stock cannot be modified without an inventory transaction.

RULE INV-002

Reprinting an ecommerce label must not deduct inventory.

RULE TENANT-001

Every business-owned database operation must be scoped
to an authenticated company.

RULE PRODUCT-001

Marketplace SKU != Product identity.

Marketplace SKUs must resolve through SKU mappings.

RULE WORKER-001

Worker assignments are configuration/data.
They must not be hardcoded in application source code.

RULE RETURN-001

A returned product must not increase sellable stock
until its return disposition permits restocking.

RULE RETURN-002

A cancellation status alone must never restore inventory. Post-outbound stock
may increase only after a physical return is received and explicitly accepted
for restock.

RULE RETURN-003

Expected and received return quantities must be explicit, must remain bounded
by the normalized order quantity, and must never default to one.
