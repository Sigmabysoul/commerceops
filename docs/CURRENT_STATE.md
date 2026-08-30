# CommerceOps Current State

## Current Phase

Phase 6 — Dashboard and Reporting

## Current Branch

`phase/06-dashboard-reporting`

## Approved Baseline

Phase 5 approved baseline:

`c573c6714d608cf2e41ba62dabfe5429479d9042`

## Phase Status

`IMPLEMENTATION_COMPLETE_AWAITING_EXTERNAL_REVIEW`

Phases 0–5 are approved. The owner approved the Phase 5 baseline and explicitly
authorized Phase 6 on 2026-08-30.

## Approved Phases

- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master
- Phase 3 — Flipkart Processing
- Phase 4 — Batch + Printing
- Phase 5 — Inventory

## Phase 6 Goal

Build a tenant-scoped operational dashboard from authoritative Product Master,
marketplace, batch/printing, and inventory records without introducing a second
source of business truth.

## Phase 6 Implementation Strategy

### Batch A — Reporting API and read queries

- [x] tenant-scoped dashboard summary query
- [x] explicit RFC3339 range and timezone-boundary semantics
- [x] marketplace breakdown and review/failure summary
- [x] inventory snapshot and ledger-derived movement
- [x] Product Master movement pagination
- [x] `reports.view` and conditional inventory authorization
- [x] reporting filter indexes without aggregate counters

### Batch B — Operational dashboard frontend

- [x] Today, Yesterday, and custom date filters
- [x] marketplace filter
- [x] large summary metrics
- [x] marketplace and queue tables
- [x] inventory snapshot and product movement table
- [x] PostgreSQL-backed end-to-end verification

## Approved Phase 5 Record

Implement one centralized, tenant-scoped inventory domain using canonical
Product Master IDs and an immutable transaction ledger. Every stock mutation
must be transactional, idempotent, authorized, auditable, and concurrency-safe.

## Phase 5 Batch Strategy

### Batch A — Ledger foundation and manual operations

- [x] immutable tenant/product inventory ledger
- [x] transactionally locked cached balances
- [x] stock-in command
- [x] manual adjustment and correction commands
- [x] negative-stock prevention
- [x] idempotency and canonical-product enforcement
- [x] inventory read APIs and transaction filters
- [x] permissions, entitlement, and audit integration
- [x] PostgreSQL concurrency, rollback, isolation, and load tests
- [x] API/OpenAPI/database documentation

### Batch B — Ecommerce outbound integration

- [x] explicit authorized outbound-confirmation trigger
- [x] atomic and idempotent `ECOMMERCE_OUT` from batch Product Master totals
- [x] entire-batch rollback on insufficient stock
- [x] explicit reprint inventory-neutral regression coverage

### Batch C — Reservation foundation and frontend

- [x] source-linked reservation lifecycle
- [x] on-hand, reserved, and available views
- [x] inventory dashboard, history, stock-in, and adjustment UI
- [x] end-to-end verification

## Approved Phase 4 Record

Build the batch and printable-output workflow on top of the normalized Flipkart processing results from Phase 3.

Phase 4 owns:

- batch creation and lifecycle
- selecting processed Flipkart jobs/orders for a batch
- printable shipping-label output
- label cropping/output normalization
- sorting of printable labels
- optional invoice separation/export
- print preparation and downloadable PDF artifacts
- traceability between generated output and source orders
- idempotent generation/re-generation where required
- user-facing batch/print UI

Phase 4 must consume normalized Phase 3 data and must not reimplement marketplace parsing.

## Required Flipkart Output Behavior

For modern Flipkart PDFs containing a shipping label and tax invoice on the same source page:

### Shipping-label output

Generate a clean printable PDF containing only the shipping-label region.

Do not include the invoice portion in the normal label output.

Generated pages should be normalized printable pages rather than relying only on the source PDF CropBox where practical.

The output must preserve barcode/QR readability and must not cover or distort required shipping information.

### Sort Labels

Provide a user-facing toggle:

`Sort Labels`

When OFF:
- preserve original source/order sequence

When ON:
- sort using the Phase 4 configured/default sorting strategy
- sorting must operate on normalized order/product metadata, not raw visual PDF text alone

Initial sorting may use Product Master / resolved SKU ordering according to the Phase 4 design.

Do not hardcode employee ownership rules into sorting.

### Export Invoices

Provide a user-facing toggle:

`Export Invoices`

When OFF:
- invoice regions are excluded/discarded from generated output
- only the shipping-label PDF is produced

When ON:
- generate two separate downloadable PDFs:
  1. shipping labels PDF
  2. invoices PDF

Invoice output should follow the same corresponding order as the generated shipping-label output where practical, so labels and invoices remain easy to associate.

The source relationship between label and invoice must not be lost.

## Output Traceability

Every generated printable item must remain traceable to:

- company
- source file
- processing job
- source page
- marketplace order
- AWB/order ID where available
- batch
- output artifact
- generation version/configuration

Generated artifacts must never create new inventory movement.

Re-generating/reprinting an existing batch must not create duplicate business effects.

## Architecture Rules

Phase 4 must preserve:

- modular monolith architecture
- strict tenant isolation
- Product Master as canonical product identity
- S3-compatible object-storage abstraction
- PostgreSQL as authoritative persistent state
- Phase 3 parser isolation
- server-side company context
- explicit permissions and module entitlement checks
- auditability
- idempotency
- bounded PDF processing

Do not place authoritative batch/printing logic in frontend components.

Do not bypass object storage by writing marketplace-specific files directly to the local filesystem.

Do not reparse Flipkart raw PDFs in printing code when normalized Phase 3 results are already available.

## Phase 4 Historical Non-goals

Do not implement:

- inventory deductions or inventory ledger functionality
- returns or cancellations
- Meesho processing
- Amazon processing
- Myntra processing
- Snapdeal processing
- consignment workflows
- supplier manifest workflows
- printer-agent/native printer integration
- hardcoded employee/product assignments
- Phase 5 functionality
- automatic progression to another phase

The Meesho labels, manifests, supplier manifests, and other sample files currently available are future reference only.

## Initial Phase 4 Implementation Strategy

Implement Phase 4 in medium-sized coherent batches rather than many tiny tasks.

Recommended sequence:

### Batch A — Batch domain and persistence
- [x] batch schema/state model
- [x] tenant-scoped batch APIs
- [x] selection of eligible processed Flipkart records
- [x] permissions/audit/idempotency foundations
- [x] PostgreSQL integration tests

### Batch B — PDF output generation
- [x] shipping-label region extraction/cropping
- [x] normalized printable label pages
- [x] Sort Labels behavior
- [x] Export Invoices behavior
- [x] separate labels/invoices artifacts
- [x] object-storage persistence
- [x] sanitized and representative production-layout regression tests

### Batch C — Frontend and end-to-end workflow
- [x] batch creation UI
- [x] sorting toggle
- [x] invoice-export toggle
- [x] processing/progress state
- [x] preview/result summary
- [x] download labels PDF
- [x] download invoices PDF when enabled
- [x] end-to-end workflow coverage through the PostgreSQL-backed batch integration suite

### Final Phase 4 Review
- [x] full PostgreSQL-backed verification
- [x] tenant isolation
- [x] idempotency/reprint safety
- [x] artifact traceability
- [x] PDF quality/barcode preservation regression coverage
- [x] owner acceptance and final static architecture check

### Batch D — Worker assignments and reprints
- [x] configurable exact-product rules and required marketplace fallback
- [x] immutable ready-batch assignment snapshots and worker totals
- [x] assignment management authorization and audit record
- [x] source-linked reprint jobs with required reason
- [x] `labels.reprint` enforcement, idempotency, history, and audit record
- [x] frontend assignment, worker-total, print-history, and reprint workflow

Do not automatically begin the next batch after completing one if the task explicitly requires review.

## Blocking Issues

None. Phase 6 intentionally reports system-side PDF readiness because
browser/native printing cannot authoritatively report physical printer completion.

## Last Verification

Approved Phase 3 baseline:

`a023a6d5087227dfe719414248d2a36e548742f7`

Phase 3 passed:

- architecture review
- production Flipkart PDF validation
- sanitized fixture regression tests
- tenant-isolation tests
- duplicate/idempotency tests
- worker-lease tests
- object-storage tests
- PostgreSQL integration tests
- backend CI
- frontend verification

These Phase 3 results are historical baseline evidence and must not be presented as verification of future Phase 4 changes. Batch A verification is recorded after the implementation has passed the required PostgreSQL-backed gate.

Phase 4 Batch A passed the full verification gate against a migrated disposable
PostgreSQL 18.6 database, including:

- Batch A migration up/down coverage
- tenant isolation, Flipkart entitlement, and `labels.process` enforcement
- eligible-order selection and duplicate membership protection
- idempotent batch creation and conflicting-key rejection
- ordered source traceability and derived Product Master totals
- unresolved-item readiness blocking and state-transition validation
- batch creation/readiness/cancellation audit records
- the full existing Go, worker-lease, object-storage, and private Flipkart PDF
  regression suites
- Go vet/build and frontend typecheck, lint, and production build
- `git diff --check`

Phase 4 Batch B passed the full PostgreSQL-backed verification gate, including:

- migration `000007` up/down validation
- tenant-scoped print jobs, items, artifacts, downloads, and `labels.print`
  enforcement
- deterministic sorted and original-order generation
- exact idempotent replay and conflicting request protection
- persisted generation failure behavior and audit coverage
- sanitized vector PDF label/invoice separation and normalized page geometry
- first-page generation across all nine representative original PDFs and all
  nine CropBox counterparts
- the complete existing Go, PostgreSQL, private Flipkart, worker-lease, and
  object-storage suites
- Go vet/build and frontend typecheck, lint, and production build
- `git diff --check`

Phase 4 Batch C and final Batch D passed the full PostgreSQL-backed repository
verification gate against a migrated disposable PostgreSQL 18.6 database,
including:

- migration `000008` up/down coverage
- tenant-scoped exact-product and fallback assignment resolution
- immutable ready-batch assignment snapshots and derived worker totals
- assignment management permission enforcement and auditability
- source-linked reasoned reprints, `labels.reprint`, exact idempotent replay,
  print history, artifact generation, and audit coverage
- client-side retry key reuse for exact batch and print requests
- the complete PostgreSQL, private Flipkart PDF, worker-lease, object-storage,
  PDF geometry, and barcode-preservation regression suites
- Go vet and backend build
- frontend typecheck, lint, and production build
- `git diff --check`

## Next Allowed Task

Perform external architecture and owner review of the complete Phase 6 working
tree. Do not begin Phase 7/Amazon without explicit authorization.

## Phase 6 Verification

Phase 6 passed the full verification gate against a freshly initialized,
migrated disposable PostgreSQL 18.6 database through migration `000011`,
including:

- tenant isolation and `reports.view` enforcement
- inventory entitlement and `inventory.view` field-level disclosure controls
- explicit timezone-offset and inclusive-start/exclusive-end date boundaries
- authoritative marketplace order, Product Master quantity, and inventory totals
- empty/restricted inventory response behavior and reporting pagination validation
- migration `000011` up/down coverage
- the complete PostgreSQL-backed Go suite, Go vet/build, frontend typecheck,
  lint, production build, and `git diff --check`
