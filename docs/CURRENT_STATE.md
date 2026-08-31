## Current Phase

Phase 7 — Amazon Processing

## Current Branch

`phase/07-amazon`

## Approved Baseline

Phase 6 approved baseline:

`2d81445d3f2a4b37fa1dd1eb464997c9b7d7934b`

## Phase Status

`BATCH_C_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–6 are approved.

Phase 7 — Amazon Processing is authorized.

Phase 8 is not authorized.

## Approved Phases

- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master
- Phase 3 — Flipkart Processing
- Phase 4 — Batch + Printing
- Phase 5 — Inventory
- Phase 6 — Dashboard and Reporting

## Phase 7 Goal

Add Amazon document processing through the existing tenant-scoped marketplace
pipeline and canonical Product Master without duplicating upload, storage, job,
batch, inventory, or reporting infrastructure.

## Phase 7 Batch Strategy

### Batch A — Amazon processing foundation

- [x] generic marketplace upload/job/lease orchestration separated from adapter parsing
- [x] isolated `amazon-text-v1` adapter boundary
- [x] secure Amazon upload, job read, and retry API integration
- [x] Amazon order ID, tracking/AWB, seller SKU, and explicit quantity foundation
- [x] exact Amazon SKU resolution through Product Master
- [x] unresolved/review behavior without guessed fields or quantity defaults
- [x] source file, source page, marketplace, and parser-version traceability
- [x] sanitized Amazon PDF and private production-structure regression coverage
- [x] full PostgreSQL-backed verification

### Batch B — OCR and deterministic document association

- [x] opt-in bounded OCR for text-empty Amazon pages
- [x] exact Amazon order-ID label/invoice association without position matching
- [x] mutually unique, evidence-complete one-page adjacency fallback
- [x] legacy-useful invoice SKU precedence including bracketed value before `HSN`
- [x] label AWB plus invoice seller SKU/explicit quantity normalization
- [x] ambiguous and incomplete associations remain review data
- [x] source file, source page, document role, and extraction-method traceability
- [x] sanitized parser/association and private production-structure regressions
- [x] tenant-scoped PostgreSQL persistence and migration up/down coverage
- [x] Amazon eligibility in shared batches and worker-assignment snapshots
- [x] validated A4 `SKU | QTY` printable enrichment through shared print artifacts
- [x] Amazon invoice artifact export plus print/reprint inventory neutrality
- [x] full PostgreSQL-backed verification

### Batch C — Inventory and reporting integration

- [x] Amazon ready batches use the central transactional ecommerce outbound confirmation
- [x] Amazon outbound confirmation is idempotent and shortage-safe
- [x] outbound event, immutable ledger, and audit traceability remain tenant/batch scoped
- [x] upload, processing, printing, and reprinting remain inventory-neutral
- [x] Amazon orders, labels, print runs, batches, quantities, and confirmed outbound appear in shared reporting
- [x] Amazon-filtered ecommerce stock-out excludes Flipkart stock-out and vice versa
- [x] company-wide current balances, stock-in, adjustments, and corrections retain their documented semantics under marketplace filters
- [x] the dashboard marketplace selector exposes Amazon
- [x] full PostgreSQL-backed verification

### Explicitly deferred after Batch C

- external architecture and owner review
- any separately authorized later Phase 7 batch
- Phase 8

## Phase 7 Batch C Verification

Batch C passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000013`, including:

- Amazon ready-batch outbound event, ledger, balance, and audit traceability
- replay-safe Amazon confirmation without duplicate deduction
- atomic rollback when an Amazon batch cannot be fully fulfilled
- Amazon-versus-Flipkart reporting isolation for orders, labels, print runs,
  batches, product quantities, confirmed orders, and ecommerce stock-out
- company-wide current balance, stock-in, and adjustment semantics under both
  marketplace filters
- all prior Amazon extraction, association, Product Master, shared batch/print,
  tenant, authorization, migration, and PDF regressions
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, and `git diff --check`

## Remaining Scope

Phase 7 Batches A through C are implemented. Any additional Phase 7 work
requires separate authorization and review. Phase 8 remains unauthorized.

## Next Allowed Task

Perform external architecture and owner review of Phase 7 Batch C. Do not begin
another Phase 7 batch or Phase 8 without explicit authorization.
