## Current Phase

Phase 8 — Returns and Cancellations

## Current Branch

`phase/08-returns-cancellations`

## Approved Baseline

Phase 7 approved baseline:

`45a15fdc3fda21ef2c0763b517068e08b0aad4f4`

## Phase Status

`BATCH_A_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–7 are approved.

Phase 8 — Returns and Cancellations is authorized.

Phase 9 is not authorized.

## Phase 8 Goal

Track marketplace cancellations and physical returns against normalized orders,
then reconcile inventory only after explicit operational evidence and an
authorized disposition.

## Phase 8 Batch Strategy

### Batch A — Cancellation and physical-return intake foundation

- [x] tenant-scoped cancellation records linked to resolved marketplace orders
- [x] authoritative pre/post-outbound snapshot without automatic restock
- [x] tenant-scoped expected returns linked to normalized order items and Product Master
- [x] explicit partial expected and received quantities without defaults
- [x] quantity bounds and concurrent intake locking
- [x] exact idempotency, append-only return events, and audit traceability
- [x] returns entitlement and dedicated view/manage/restock permissions
- [x] shared Flipkart/Amazon domain and API contracts
- [x] full PostgreSQL-backed verification

## Phase 8 Batch A Verification

Batch A passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000014`, including:

- Flipkart and Amazon normalized-order association
- cancellation classification before and after authoritative outbound confirmation
- exact replay, changed-payload conflict, and duplicate cancellation prevention
- explicit partial expected and received quantities without quantity defaults
- cumulative and concurrent return-quantity bounds against the original order
- cross-tenant read/write isolation and returns entitlement/permission denial
- append-only event and audit traceability
- confirmation that cancellation, intake, and receipt create no inventory transaction
- migration `000014` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, and `git diff --check`

### Deferred after Batch A

- cancelled-order dispatch prevention and closure workflow
- inspection and disposition actions
- centralized `return_restock` inventory integration and compensating correction
- frontend queues and return/cancellation detail screens
- Phase 6 dashboard return/cancellation metrics
- later Phase 8 batches and Phase 9

## Next Allowed Task

Complete and externally review Phase 8 Batch A. Do not begin its inventory
integration, later Phase 8 batches, or Phase 9 automatically.
