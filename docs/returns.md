# Returns and cancellations — Phase 8 Batch A

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

## Inventory boundary

Batch A performs no inventory mutation. Expected, cancelled, or physically
received status is not a restock decision. Later Phase 8 work must add explicit
inspection/disposition and call the centralized inventory domain only after an
authorized restockable decision. It must use `return_restock` ledger entries,
prevent duplicate restoration, and use compensating corrections rather than
rewriting history.

## Deferred after Batch A

- preventing later dispatch of a pre-outbound cancelled order
- inspection and restockable/damaged/rejected disposition actions
- centralized inventory restock and correction integration
- cancellation/return closure workflows
- frontend queues and detail screens
- dashboard return and cancellation metrics
