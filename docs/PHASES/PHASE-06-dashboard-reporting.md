Implement CommerceOps Phase 6 — Dashboard and Reporting.

Do not begin until Phase 5 Inventory has passed its review gate and has an approved baseline.

Read all architecture, Product Master, marketplace, batch/printing and inventory documentation first.

## Goal

Build a trustworthy operational dashboard using existing authoritative domain data.

Reporting must not become a second source of business truth.

Do not maintain independent dashboard counters when the value can be derived from authoritative records.

## Dashboard overview

Provide company-scoped metrics for configurable date ranges.

Initial dashboard should show:

* ecommerce orders processed
* labels generated
* labels/print runs completed
* batches
* outbound-confirmed orders
* stock out
* stock in
* current inventory
* unresolved marketplace records
* duplicate/review records
* failed processing jobs

Provide marketplace breakdown where applicable.

## Daily operational summary

Support:

Today
Yesterday
Custom date

Show:

* total orders
* quantity by product
* stock-out quantity
* stock-in quantity
* net inventory movement
* batches generated
* failed/review-required items

Product totals must use canonical Product Master identity.

## Marketplace breakdown

Initial marketplace:

Flipkart

Architecture must allow later:

Amazon
Meesho
Myntra
Snapdeal

Do not hardcode dashboard structure in a way that requires rewriting the reporting system for each marketplace.

## Worker/operator reporting

Do not hardcode employee names or SKU prefixes.

If worker assignment information exists from approved earlier phases, reporting may group activity by configured employee/assignment.

Otherwise keep worker views isolated behind reusable reporting structures.

## Inventory reporting

Inventory metrics must derive from Inventory transactions/balances.

Support:

* current on-hand
* reserved
* available
* stock-in during range
* ecommerce stock-out during range
* adjustments
* product movement

Do not recalculate historical stock by interpreting marketplace PDFs.

## Reporting architecture

Prefer explicit reporting queries/read models.

Avoid:

* N+1 queries
* loading whole ledgers into memory
* duplicated business rules
* materialized counters without reconciliation

Indexes should match real reporting filters.

If aggregated read models are introduced, document:

* source of truth
* refresh/update mechanism
* rebuild/reconciliation strategy

## Frontend

Build clear operational dashboard screens suitable for everyday workplace use.

Favor large readable numbers and simple tables.

Include:

* dashboard summary
* product movement table
* marketplace breakdown
* inventory snapshot
* review/failure queue summary

Filters:

* date range
* marketplace
* product where useful

Do not build complex BI/chart-builder functionality.

## Export

Allow basic CSV export for useful operational tables if it fits existing architecture.

Do not build accounting exports or GST/financial statements.

## Authorization

All dashboard data must be company-scoped.

Respect module entitlements and permissions.

Users without inventory access must not receive restricted inventory data merely because a dashboard endpoint combines metrics.

## Performance

Test realistic data volumes.

Inspect SQL query plans where warranted.

Report obvious indexing or query-performance issues.

## Tests

Test:

* tenant isolation
* date boundaries
* timezone handling
* totals against authoritative transactions
* marketplace totals
* inventory totals
* unresolved/duplicate counts
* permission enforcement
* empty periods
* pagination where needed
* PostgreSQL reporting queries

Update docs and `CURRENT_STATE.md`.

Run full verification.

STOP after Phase 6.

Do not begin Amazon/Phase 7.
