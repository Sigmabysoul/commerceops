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

## Phase 23 operations and workforce analytics

`GET /api/v1/reports/operations-analytics` is a separate company-scoped read model. Access
requires `reports.view`, `traceability.view` and the `traceability` entitlement together.
Trace Box records have no authoritative marketplace ownership, so this report has no
marketplace filter. Reporting does not modify operational records or Inventory.

Clients send `from` and `to` RFC3339 instants with the same `[from,to)` range and 366-day maximum.
`timezone` is a validated IANA name, default `UTC`, used only to group daily inspections.
The web client sends browser-local midnight boundaries and the browser's timezone, including
daylight-saving offsets. Only days with inspections appear; a missing day is not a measured
zero-defect day. `limit` (1–100, default 20) and `offset` (0–1,000,000) page workforce rows only.

The `traceability-v1` definitions are:

| Measure | Authoritative formula and cohort |
|---|---|
| Checks / inspected units | Distinct QC events / sum of their snapshot checked quantities, using QC event time in the range. Multi-Product checks count once. |
| Rejected units / rejection rate | Sum of rejected inspection quantities / that sum divided by all inspected units times 100. Repeated QC contributes additional inspection workload. |
| Daily trends | The same inspection measures grouped by event instant converted to the selected local date. |
| Defect reason share | Rejected snapshot units for each reason divided by all rejected inspection units in the range times 100. |
| Rework elapsed time | Completion event minus its requirement's originating QC event; the completion is in the range, even if QC preceded it. |
| Handover elapsed time | Receipt event minus its referenced send event; the receipt is in the range. Pending transfers have no completed duration. |
| First shipment-readiness time | First-ever readiness event minus box creation. Only boxes whose first readiness is in the range contribute; later re-readiness does not add another sample. |
| Workforce QC | The same checked/rejected workload grouped by the recorded checking employee. The employee detected the rejection; this does not identify who caused it. |
| Workforce completed work | Completed requirement count and quantity, grouped by recorded completing employee, using completion event time. Completion is not proof of a later passing QC. |
| Workforce final checks | Count of typed final checks and failed checks, with failures divided by all final checks times 100, grouped by recorded checking employee and final-check event time. |

Cycle durations report sample count, mean, median and p95 elapsed hours. Durations include
waiting; they do not measure labor hours. Empty denominators and no completed samples produce
JSON `null` rates/durations, displayed as “No sample”. Activity counts are zero for empty cohorts.

Each response is computed in one read-only repeatable-read transaction with a 15-second query
budget, and returns `generated_at`, `metric_version`, range and timezone. Immutable source
history makes the metrics recomputable. Employee names are current display labels; employee
IDs retain attribution even after deactivation. Workforce rows use employee-ID order, and total
counts cover the full filtered population even when the requested page is empty. Separate page
requests take new snapshots, so new in-range activity may change a page population.

There is no reliable producer attribution, clocked labor time, per-unit identity, or historical
department context for these QC events. Consequently the report does not infer employee-caused
defects, units per labor hour, unique defective units, department rankings, or rework success.
Activity types remain separate. Gamification requires a separately approved later design.
