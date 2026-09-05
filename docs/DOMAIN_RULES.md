RULE INV-001

Stock cannot be modified without an inventory transaction.

RULE INV-002

Reprinting an ecommerce label must not deduct inventory.

RULE PRINT-001

Generating, queueing, printing, cancelling, failing, or retrying a print job is
inventory-neutral. Physical printing must never create an inventory transaction.

RULE PRINT-002

An agent may submit only a server-authorized immutable PDF to a printer that it
reported and the tenant registered. Agent APIs must never expose arbitrary
commands, paths, storage keys, or printer options.

RULE PRINT-003

Automatic reconnect recovery must not resubmit a job that crossed the durable
local submission boundary. Ambiguous failures require an explicit audited retry.

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

RULE RETURN-004

Only explicitly inspected `restockable` quantities may create a
`return_restock` inventory transaction. Damaged, rejected, wrong-product, and
missing quantities must not increase sellable stock.

RULE RETURN-005

An incorrect return restock must be reversed with an immutable compensating
inventory correction bounded by the original restocked quantity.

RULE CANCELLATION-001

A pre-outbound cancellation must prevent later batch readiness and inventory
outbound confirmation. A post-outbound cancellation must not silently reverse
the committed deduction.

RULE RETURN-006

Closing a return or cancellation is a lifecycle operation only. It must append
actor/time history, must be idempotent, and must never create or reverse an
inventory transaction. Returns with incomplete receipt, inspection, or an
unapplied restockable disposition cannot be closed.

RULE CONSIGNMENT-001

Consignment product identity and quantities must use canonical tenant Product
Master records. Departments are tenant configuration and must not be inferred
from employee names or hardcoded examples.

RULE CONSIGNMENT-002

Allocation reserves required stock and reduces Available only. On-hand changes
only when a fully packed consignment is explicitly confirmed outbound through
Inventory, producing immutable `CONSIGNMENT_OUT` ledger entries.

RULE CONSIGNMENT-003

Ready and packed quantities are explicit and bounded by the required quantity.
A consignment cannot become ready, packed, outbound, or completed while any
required line is incomplete.

RULE CONSIGNMENT-004

Pre-outbound cancellation releases active reservations without an inventory
ledger movement. Outbound and completed consignments cannot be cancelled, and
replaying outbound confirmation must never deduct stock twice.

RULE CONSIGNMENT-005

Pouch/file references are traceability fields, not globally unique identities,
unless a future approved workflow establishes and migrates a narrower
uniqueness scope.
