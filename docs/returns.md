# Returns and cancellations — Phase 8

The `internal/returns` domain owns cancellation records and physical-return
lifecycle data. It reuses normalized marketplace orders, canonical Product
Master items, centralized tenant authorization, module entitlements, audit
logging, and PostgreSQL transactions. Flipkart and Amazon share this domain;
marketplace-specific return ingestion remains outside it.

## Cancellation foundation

A cancellation references one tenant-owned, resolved marketplace order. At
recording time the service checks the authoritative batch outbound event and
stores one of:

- `not_outbound`: no ecommerce stock deduction exists, so cancellation records
  no inventory movement.
- `outbound_confirmed`: a stock deduction exists, but cancellation alone still
  does not prove physical receipt or restockability and records no inventory
  movement.

One cancellation record is allowed per order. Exact idempotent retries return
the original record; changed payloads and duplicate business events conflict.

## Return intake foundation

A return case references one resolved marketplace order. Every return item
references a resolved normalized order item and its canonical Product Master
product. Expected quantities are explicit, may be partial, and cannot exceed
the original order quantity across all return cases. Order and item rows are
locked while those bounds are checked, so concurrent intake cannot over-claim
eligible quantity.

`expected` cases may be marked `received` exactly once with an explicit
quantity for every item, including zero when an expected item is missing.
Received quantity cannot exceed expected quantity. Create and receive actions
use request hashes and tenant-scoped idempotency keys. Append-only events and
audit records preserve actor and timestamp history.

## Inspection and inventory boundary

Expected, cancelled, and physically received states remain inventory-neutral.
An inspection records one explicit disposition for every return line. A
zero-received line must be `missing`; a positive received line cannot be
`missing`. Damaged, rejected, wrong-product, and missing lines never increase
sellable inventory.

Only an `inspected` case containing positive `restockable` quantities may be
restocked. The returns domain owns that workflow decision, while the Inventory
domain owns balance locking, immutable `return_restock` ledger entries, and
inventory audits. Event, return state, per-item restocked quantity, ledger, and
balance update commit atomically. The operation requires both the `returns` and
`inventory` entitlements plus `returns.restock`.

An incorrect restock is reversed only through a bounded compensating
`correction` ledger entry. Original restock transactions and events remain
immutable. Cumulative corrections cannot exceed the quantity previously
restocked for a return item.

## Cancellation dispatch safety

Orders cancelled before outbound are excluded from batch eligibility and new
batch creation. A draft containing a subsequently cancelled order cannot become
ready, and outbound confirmation rejects it. Order-row locks serialize
cancellation with readiness/outbound: either outbound commits first and the
cancellation records `outbound_confirmed`, or cancellation commits first and no
stock deduction occurs. Replaying an outbound that already committed remains
safe after a post-outbound cancellation.

## Closure

Cancellation closure is allowed from `recorded`. Return closure is allowed only
from `restocked`, `restock_corrected`, `damaged`, or `rejected`; incomplete
receipt, inspection, and unapplied restockable decisions cannot be hidden by
closing early. Both close actions require `returns.manage`, use exact
idempotency, record actor/time and append-only history, and are inventory-neutral.

## Operator workspace

The typed frontend provides expected, received, needs-inspection, and completed
return queues plus cancellation records. Detail views show the normalized source
order, canonical products and quantities, disposition, status history, and every
linked inventory movement. Stock-changing restock/correction actions require an
explicit confirmation; closure clearly states that it does not change stock.

## Reporting

The inventory dashboard exposes net `return_restock` movement, including its
linked compensating corrections. Marketplace filters follow the normalized
source order. With `returns.view`, the dashboard also derives cancellation,
receipt, received quantity, gross restock, damaged quantity, and closure metrics
from authoritative Phase 8 records. The cohort return rate is resolved source
orders created in the selected range that have a physically received return,
divided by all resolved source orders in that range. Without the returns
entitlement/permission, these fields are omitted.
