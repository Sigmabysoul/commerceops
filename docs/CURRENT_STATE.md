## Current Phase

Phase 7 — Amazon Processing

## Current Branch

`phase/07-amazon`

## Approved Baseline

Phase 6 approved baseline:

`2d81445d3f2a4b37fa1dd1eb464997c9b7d7934b`

## Phase Status

`BATCH_A_COMPLETE_AWAITING_EXTERNAL_REVIEW`

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

### Explicitly deferred after Batch A

- final shipping-label/invoice association
- OCR for image-only Amazon shipping-label pages
- Amazon batch/print output manipulation
- inventory integration changes
- Phase 7 Batch B and Phase 8

## Phase 7 Batch A Verification

Batch A passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000012`, including:

- Amazon entitlement plus `labels.upload` / `labels.process` authorization
- tenant isolation and tenant/marketplace source-file deduplication
- marketplace-scoped durable worker claims and lease-safe persistence
- exact active Amazon SKU resolution through Product Master
- missing and ambiguous field review behavior without quantity defaults
- source file, source page, marketplace, and `amazon-text-v1` traceability
- retry after SKU training and duplicate Amazon order visibility
- sanitized two-page Amazon PDF extraction
- private ten-page production structure validation without committing or logging identifiers
- migration `000012` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, and `git diff --check`

## Blocking Issues

The supplied production shipping-label pages are image-only and expose no text
through the approved Poppler boundary. OCR and deterministic label/invoice
association require a separately reviewed later batch. Batch A records only
explicit text-extractable values and does not position-match alternating pages.

## Next Allowed Task

Perform external architecture and owner review of Phase 7 Batch A. Do not begin
Batch B or Phase 8 without explicit authorization.
