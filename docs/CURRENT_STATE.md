## Current Phase

Phase 7 — Amazon Processing

## Current Branch

`phase/07-amazon`

## Approved Baseline

Phase 6 approved baseline:

`2d81445d3f2a4b37fa1dd1eb464997c9b7d7934b`

## Phase Status

`BATCH_B_COMPLETE_AWAITING_EXTERNAL_REVIEW`

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

### Explicitly deferred after Batch B

- inventory integration changes
- Amazon reporting inclusion verification
- later Phase 7 batches and Phase 8

## Phase 7 Batch B Verification

Batch B passed `make verify-full` against a freshly initialized disposable
PostgreSQL 18.6 database migrated through `000013`, including:

- Amazon entitlement plus `labels.upload` / `labels.process` authorization
- tenant isolation and tenant/marketplace source-file deduplication
- marketplace-scoped durable worker claims and lease-safe persistence
- exact active Amazon SKU resolution through Product Master
- missing and ambiguous field review behavior without quantity defaults
- source file, source page, marketplace, document role, extraction method, and
  `amazon-associated-v3` traceability
- retry after SKU training and duplicate Amazon order visibility
- sanitized two-page Amazon PDF extraction
- exact order-ID association and ambiguous-association review behavior
- validated/rejected adjacency and legacy-useful invoice SKU precedence
- shared Amazon batch/printing, artifact traceability, and inventory-neutral reprints
- sanitized and private A4 enriched-output PDF regression checks
- private ten-page OCR/association validation without committing or logging identifiers
- migration `000013` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, and `git diff --check`

## Remaining Scope

The existing outbound inventory event integration and Phase 6 reporting
inclusion still require separately reviewed later Phase 7 batches.

## Next Allowed Task

Perform external architecture and owner review of Phase 7 Batch B. Do not begin
another Phase 7 batch or Phase 8 without explicit authorization.
