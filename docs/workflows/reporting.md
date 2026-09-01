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

When a marketplace filter is active, `ecommerce_out` transactions are included
only when their immutable batch reference belongs to that marketplace.
`return_restock` and its linked compensating corrections follow their return's
normalized source-order marketplace. Stock-in, manual adjustments, and general
corrections remain company-wide because those ledger events do not belong to a
marketplace. Current on-hand, reserved, and available also remain company-wide
live snapshots; they are not marketplace-owned balances.

Return/cancellation operational metrics require the returns entitlement and
`returns.view`. Cancellation occurrence uses `cancelled_at`; receipt,
inspection, restock, and closure use append-only event times. Gross restocked
quantity remains an operational count while Inventory `return_restock` reports
the net ledger effect after corrections. Cohort return rate uses resolved source
orders created in the selected range and is explicitly not an accounting or
profitability metric.

Consignment reporting requires the `consignments` entitlement plus broad
`consignments.view`/`consignments.manage` or scoped `consignments.work` access.
It derives pending work, range-completed work, Product Master requirements,
department workload, average completion time, and `consignment_out` movement
from authoritative records. Department workers see only assigned line totals;
aggregate outbound movement is omitted for scoped workers because a single
product reservation can span departments and cannot be divided reliably after
aggregation.

Consignments are company operations rather than marketplace-owned orders.
Therefore `consignment_out`, like stock-in and general adjustments, remains in
inventory movement when a marketplace filter is active. Marketplace filtering
continues to isolate only ecommerce and return movements whose authoritative
source records carry a marketplace.

Every query includes the authenticated company ID. `reports.view` gates the
dashboard. Inventory fields require both the inventory module entitlement and
`inventory.view`; lacking either produces a useful non-inventory report rather
than leaking restricted values.
