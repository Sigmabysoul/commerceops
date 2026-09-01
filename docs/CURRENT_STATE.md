## Current Phase

Phase 10 — Meesho

## Current Branch

`phase/10-meesho`

## Approved Baseline

Phase 9 completed baseline explicitly advanced by the owner for Phase 10:

`69fa09c8`

## Phase Status

`PHASE_10_BATCH_A_COMPLETE_AWAITING_REVIEW`

Phases 0–9 are implemented. The owner explicitly authorized Phase 10. Phase 10
is being delivered in medium cohesive batches; Batch A establishes Meesho
processing without prematurely adding printing or inventory behavior.

Phase 11 is not authorized.

## Phase 10 Goal

Add Meesho as a first-class marketplace while reusing generic secure upload,
durable processing, Product Master, review, duplicate, traceability, batch,
printing, inventory, reporting, and returns infrastructure. Marketplace-specific
document recognition and output rules remain isolated in the Meesho adapter.

## Batch A Delivery

### Processing foundation

- [x] isolated `meesho-labeled-v1` adapter using bounded Poppler page text
- [x] explicit sub-order/order ID, AWB/tracking, supplier/seller SKU, and positive quantity extraction
- [x] sub-order precedence and conservative multi-signal shipping-label recognition
- [x] missing, zero, malformed, or ambiguous fields persisted for review without guessing or quantity defaults
- [x] exact active Meesho Product Master resolution and safe reprocessing after SKU training
- [x] source file, source page, extraction method, parser version, job, order, and item traceability

### Shared platform integration

- [x] existing PDF validation, object storage, tenant keys, hash deduplication, PostgreSQL leases, workers, permissions, entitlement, and audit infrastructure
- [x] marketplace-isolated worker claims, duplicate-source behavior, duplicate business-identifier review, and tenant-safe reads/retries
- [x] inventory-neutral upload and processing
- [x] `/api/v1/meesho/jobs` upload and tenant-scoped job read/retry endpoints
- [x] reusable typed marketplace-processing client/view shared with Flipkart plus a Meesho operator panel
- [x] migration `000018` limited to a Meesho queued-job claim index; no Meesho business tables

### Regression coverage

- [x] sanitized structural Meesho label fixture and parser regressions
- [x] optional private-PDF regression hook through `MEESHO_PRIVATE_SAMPLE`
- [x] PostgreSQL coverage for entitlements, tenant isolation, Product Master resolution, retries, source/business duplicates, traceability, marketplace worker isolation, and inventory neutrality
- [x] migration `000018` up/down coverage

## Deliberately Deferred from Batch A

- Meesho OCR or deterministic cross-page association, pending representative private samples
- shared batch membership and printable artifact generation
- outbound confirmation and inventory ledger integration
- reporting and returns integration
- Phase 11

## Verification

Batch A passed the complete PostgreSQL-backed verification path against a
disposable database migrated through `000018`, including Go tests/vet/build,
frontend typecheck/lint/production build, OpenAPI parsing, migration coverage,
and `git diff --check`.

The optional private Meesho PDF test was skipped because
`MEESHO_PRIVATE_SAMPLE` was not supplied. Sanitized parser and full platform
regressions passed.

## Next Allowed Task

Review Phase 10 Batch A. A separately authorized Batch B may add evidence-based
Meesho association/print support and shared batch participation. Do not begin
Batch B or Phase 11 automatically.
