## Current Phase

Phase 9 — Consignment Management

## Current Branch

`phase/09-consignment`

## Approved Baseline

Phase 8 implementation baseline explicitly advanced by the owner for Phase 9:

`1cb661a5a13f786ec979fca1381c4849e8089f5e`

## Phase Status

`PHASE_9_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–7 are approved. Phase 8 was completed at the baseline above, and the
owner explicitly authorized and requested complete Phase 9 implementation.

Phase 10 is not authorized.

## Phase 9 Goal

Digitize company consignment work from manual/imported SO requirements through
configurable department preparation, packing, inventory-safe outbound, and
completion with Product Master, employee, permission, audit, and reporting
integration.

## Phase 9 Delivery

### Domain and workflow

- [x] tenant-scoped consignment/SO records with manual/import source traceability
- [x] company-unique order reference and non-globally-unique pouch/file reference
- [x] configurable departments, lifecycle status, and active employee membership
- [x] permission-based broad management and employee-backed department work views
- [x] explicit `created → allocated → picking → ready → packing → packed → outbound → completed` state machine
- [x] safe pre-outbound cancellation and invalid/backward transition rejection
- [x] canonical Product Master lines with explicit required, ready, and packed quantities
- [x] strict `packed ≤ ready ≤ required` bounds and full-completion gates
- [x] independent line versions for concurrent workers and state expected versions
- [x] exact idempotency, immutable events, actors/timestamps/notes, and shared audit logging

### Inventory and reporting

- [x] atomic Inventory-owned reservation on allocation without changing On-hand
- [x] atomic cancellation release without a stock ledger movement
- [x] atomic outbound reservation consumption and immutable `CONSIGNMENT_OUT`
- [x] duplicate/concurrent outbound protection with one company/source/product ledger entry
- [x] pending, completed-range, completion-time, Product Master quantity, department workload, and inventory-movement reporting
- [x] department-scoped reporting without cross-department quantity/event leakage
- [x] documented company-wide `consignment_out` semantics under marketplace filters

### API and UI

- [x] REST contracts for department configuration/membership, board/detail, create, allocate, progress, transition, outbound, and cancellation
- [x] typed frontend client and readable consignment board/detail workspace
- [x] multi-product SO entry, department/state/reference filters, progress controls, and confirmed inventory-changing actions
- [x] consignment summaries, department workload, and product/ledger movement in the existing dashboard
- [x] OpenAPI 0.9.0 and domain/module/database/workflow documentation

## Phase 9 Verification

Phase 9 passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000017`, including:

- canonical Product Master and tenant-composite database constraints
- department membership visibility and cross-department reporting isolation
- complete state transitions, partial-quantity guards, and stale-line conflicts
- concurrent updates to the same line with exactly one accepted version
- reservation creation, Available-only allocation effect, and cancellation release
- fully packed outbound deduction, exact replay, ledger uniqueness, and completion
- immutable consignment events and shared Consignment/Inventory audit records
- company-wide consignment movement under a marketplace-filtered dashboard
- migration `000017` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, OpenAPI YAML parsing, and `git diff --check`

## Next Allowed Task

Externally review and approve Phase 9. Do not begin Phase 10 or Meesho
automatically.
