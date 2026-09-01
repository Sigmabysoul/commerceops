## Current Phase

Phase 8 — Returns and Cancellations

## Current Branch

`phase/08-returns-cancellations`

## Approved Baseline

Phase 7 approved baseline:

`45a15fdc3fda21ef2c0763b517068e08b0aad4f4`

## Phase Status

`PHASE_8_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–7 are approved.

Phase 8 — Returns and Cancellations is authorized.

Phase 9 is not authorized.

## Phase 8 Goal

Track marketplace cancellations and physical returns against normalized orders,
then reconcile inventory only after explicit operational evidence and an
authorized disposition.

## Phase 8 Delivery

### Batch A — Cancellation and physical-return intake foundation

- [x] tenant-scoped cancellation records linked to resolved marketplace orders
- [x] authoritative pre/post-outbound snapshot without automatic restock
- [x] tenant-scoped expected returns linked to normalized order items and Product Master
- [x] explicit partial expected and received quantities without defaults
- [x] quantity bounds and concurrent intake locking
- [x] exact idempotency, append-only return events, and audit traceability
- [x] returns entitlement and dedicated view/manage/restock permissions
- [x] shared Flipkart/Amazon domain and API contracts
- [x] full PostgreSQL-backed Batch A verification

### Disposition and inventory integration

- [x] explicit inspection and restockable/damaged/rejected/wrong/missing dispositions
- [x] centralized, atomic `return_restock` inventory integration
- [x] bounded immutable compensating restock corrections
- [x] cancellation-aware batch eligibility, readiness, and outbound confirmation
- [x] exact and concurrent replay protection with full ledger reconciliation

### Phase completion

- [x] explicit inventory-neutral return and cancellation closure
- [x] closure actor/timestamp, append-only history, idempotency, and audit
- [x] expected, received, needs-inspection, completed, and cancellation queues
- [x] detail actions, Product Master quantities, inventory impact, and history
- [x] permission-gated return/cancellation reporting and defined cohort return rate
- [x] Flipkart/Amazon marketplace filtering and tenant isolation
- [x] final full PostgreSQL-backed verification

## Phase 8 Verification

Phase 8 passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000016`, including:

- pre/post-outbound cancellation behavior and dispatch race serialization
- explicit partial receipt, inspection, restock, correction, and closure
- exact replay, changed-payload conflict, and concurrent action bounds
- Flipkart/Amazon association, marketplace filtering, and tenant isolation
- authorization and module-entitlement denial without reporting leakage
- gross operational return metrics, cohort rate, and net ledger reconciliation
- migrations `000014`, `000015`, and `000016` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, OpenAPI parsing, and `git diff --check`

## Next Allowed Task

Externally review and approve Phase 8. Do not begin Phase 9 automatically.
