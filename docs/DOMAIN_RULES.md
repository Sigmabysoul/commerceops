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

RULE AUTOMATION-001

Schedules and approved domain events create normal Printing jobs only. They
never contact printers, infer events from browser state, generate stock
movements, or automatically retry ambiguous physical delivery.

RULE AUTOMATION-002

A rule occurrence produces at most one initial physical job across restarts and
concurrent workers. Batch/Consignment facts commit with their source transition;
queue creation commits with its execution outcome. Rules and every referenced
asset, printer, event, and job must belong to the same tenant.

## Planning invariants for Phases 16–25

These supplement existing rules; they do not imply new Phase 15 behavior.

RULE COMPANY-001: Single-business-first never permits removing company safety scope.
RULE SELLER-001: Seller accounts/trading identities are separate from workstations and printer agents.
RULE DEPARTMENT-001: Reuse canonical departments. One active Product assignment; changes preserve historical and in-flight context.
RULE TRACE-001: Trace QR/barcodes identify opaque server records, never encoded mutable workflow state or a second stock balance.
RULE TRACE-002: Trace Box contents use canonical Product IDs and explicit positive quantities. Every addition or removal is immutable and idempotent; concurrent removal must never make a derived quantity negative.
RULE TRACE-003: Trace Box custody identifies exactly one same-company employee or department. Current custody is derived from immutable transfer history, and scanning an identifier never bypasses authentication or authorization.
RULE QC-001: QC PASS is not RESTOCK. Sellable stock changes only through an explicit authorized Inventory transition.
RULE QC-002: Trace Box QC covers the full current Product/quantity snapshot. Rejected quantities require a reason and work type; completed work requires a fresh passing QC before packing.
RULE TRACE-004: A handover is in transit after send and changes custody only when its target employee or department member receives it. Other box mutations are blocked in transit.
RULE PACKING-001: Packing requires clear work and fresh passing QC. Shipment readiness requires a later passing final check; every gate is idempotent and audited.
RULE CONSIGNMENT-006: Traceability-required Consignment progress and outbound require current verified Trace Box quantity for every line. Allocations are immutable, bounded across consignments, and use each line's stored Product and department snapshot.
RULE CONSIGNMENT-007: Pouch and file evidence values are company-scoped trace references and are never assumed unique identifiers.
RULE LIFECYCLE-001: No automatic archival deletion before verified export and restore; retain audit history.
RULE REPORT-001: Operations analytics derives from immutable recorded events in the authenticated company. Both Reporting and Traceability permissions and the Traceability entitlement are required.
RULE REPORT-002: QC rejection rates use inspected-unit denominators, count repeat inspections as repeat workload, and attribute observation to the recorded inspector without inferring defect causation. Missing denominators are null, not measured zero.
RULE REPORT-003: Completed-cycle cohorts use completion instants and their authoritative source starts, including starts before the range. First shipment readiness contributes at most one duration per box. Elapsed time includes waiting and is not labor time.
