## Current Phase

Phase 10 — Meesho

## Current Branch

`phase/10-meesho`

## Approved Baseline

Phase 10 Batch A processing foundation:

`17b85018dec8983be98f1a1b8a6cb44bdae39088`

## Phase Status

`PHASE_10_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–9 are implemented. The owner explicitly authorized completion of all
remaining Phase 10 work from the approved Batch A foundation.

Phase 11 is not authorized.

## Phase 10 Delivery

### Meesho adapter and processing

- [x] isolated `meesho-labeled-v1` adapter using bounded Poppler page text
- [x] explicit sub-order/order ID, AWB/tracking, supplier/seller SKU, and positive quantity extraction
- [x] conservative multi-signal recognition with no inferred values or quantity default
- [x] unresolved/review behavior for missing, malformed, conflicting, duplicate, and unknown Product Master values
- [x] source file, source page, extraction method, parser version, job, normalized order, and item traceability
- [x] generic secure upload, object storage, tenant keys, source deduplication, PostgreSQL leases, permissions, entitlement, retry, and audit behavior

### Shared batch and printing

- [x] Meesho eligible-order listing, idempotent batch creation, Product Master totals, readiness, and cancellation
- [x] configurable fallback/exact-product worker assignments snapshotted at readiness
- [x] generic `source-page-v1` artifact generation preserving the complete shipping-label source page
- [x] deterministic sorting, tenant-scoped downloads, print history, and traceable idempotent reprints
- [x] explicit rejection of unsupported Meesho invoice export instead of guessed association or geometry
- [x] typed frontend marketplace selector for Flipkart, Amazon, and Meesho batch operations

### Inventory, reporting, and returns

- [x] upload, parsing, batching, printing, and reprinting remain inventory-neutral
- [x] central ready-batch confirmation creates atomic, immutable, idempotent Meesho `ecommerce_out` entries
- [x] dashboard Meesho filter includes only Meesho orders/batches/print/outbound and Meesho-linked return movement
- [x] company-wide current balances, stock-in, general adjustment/correction, and consignment semantics remain unchanged under the Meesho filter
- [x] shared cancellation and physical-return workflows accept resolved Meesho orders and explicit quantities
- [x] dashboard, returns, and batch operator selectors include Meesho

### Database and API

- [x] migration `000018` remains the only Phase 10 migration and adds only the Meesho claim index
- [x] no Meesho-specific business, batch, print, inventory, reporting, or returns tables
- [x] existing batch/assignment API marketplace contracts expanded to Meesho; no duplicate endpoints
- [x] OpenAPI 0.10.0 and module/architecture/database/workflow documentation updated

## Evidence-based Limits

Representative private Meesho PDFs were not supplied. Phase 10 therefore does
not invent OCR, cross-page association, crop coordinates, overlays, or invoice
matching. The private sample regression hook remains available through
`MEESHO_PRIVATE_SAMPLE`; future layout-specific behavior requires representative
evidence and sanitized regression fixtures.

## Verification

Phase 10 passed the complete PostgreSQL-backed `make verify-full` path against
a fresh disposable PostgreSQL 18.6 database migrated through `000018`. Coverage
includes parser behavior, tenant/permission/entitlement enforcement, Product
Master resolution, duplicates and retry, batch/assignment/print traceability,
print/reprint neutrality, outbound idempotency, reporting isolation, Returns
compatibility, migration up/down, Go vet/build, frontend typecheck/lint/build,
OpenAPI parsing, and repository checks.

Private Meesho production-PDF validation was skipped because
`MEESHO_PRIVATE_SAMPLE` was not supplied. Sanitized and platform regressions
passed.

## Next Allowed Task

Externally review and approve Phase 10. Do not begin Phase 11 or Myntra
automatically.
