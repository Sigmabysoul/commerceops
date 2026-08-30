# Dashboard and reporting workflow

Phase 6 reporting is a read-only projection over authoritative Product Master,
marketplace, batch/printing, and inventory records. It does not persist counters
or reinterpret uploaded PDFs.

Clients send an inclusive `from` and exclusive `to` RFC3339 instant. Today and
Yesterday are calculated in the operator's local timezone and retain their UTC
offset when serialized. The maximum range is 366 days.

Orders and review states use normalized marketplace-order creation time. Batch
counts use batch creation time. Completed print runs and generated label pages
use successful print completion. Outbound-order counts use the immutable
outbound event and its batch membership. Stock movement uses inventory ledger
transaction time; current on-hand, reserved, and available values are live
balance snapshots and are not historical-range reconstructions.

Every query includes the authenticated company ID. `reports.view` gates the
dashboard. Inventory fields require both the inventory module entitlement and
`inventory.view`; lacking either produces a useful non-inventory report rather
than leaking restricted values.
