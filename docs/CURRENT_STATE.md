# CommerceOps Current State

## Current Phase

Phase 4 — Batch + Printing

## Current Branch

`phase/04-batch-printing`

## Approved Baseline

Phase 3 approved baseline:

`a023a6d5087227dfe719414248d2a36e548742f7`

## Phase Status

`IMPLEMENTATION_IN_PROGRESS`

Phase 3 — Flipkart Processing is approved.

Phase 4 work may now begin, but only within the approved Phase 4 scope.

Phase 4 Batch A implementation is complete in the working branch and awaits
review. It adds the batch persistence/domain foundation only; Batch B and Batch
C have not started.

## Approved Phases

- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master
- Phase 3 — Flipkart Processing

## Phase 4 Goal

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

## Explicitly Forbidden Work

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
- shipping-label region extraction/cropping
- normalized printable label pages
- Sort Labels behavior
- Export Invoices behavior
- separate labels/invoices artifacts
- object-storage persistence
- regression tests using sanitized Flipkart fixtures

### Batch C — Frontend and end-to-end workflow
- batch creation UI
- sorting toggle
- invoice-export toggle
- processing/progress state
- preview/result summary
- download labels PDF
- download invoices PDF when enabled
- end-to-end tests

### Final Phase 4 Review
- full verification
- tenant isolation
- idempotency/reprint safety
- artifact traceability
- PDF quality/barcode preservation
- external architecture review

Do not automatically begin the next batch after completing one if the task explicitly requires review.

## Blocking Issues

Batch A awaits review before Batch B begins. PDF output generation, print jobs,
sorting/assignment behavior, reprinting, and the frontend workflow remain Phase
4 work.

No Phase 5 work is authorized.

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

## Next Allowed Task

Review Phase 4 Batch A. Do not begin Batch B until the owner explicitly
authorizes it after reviewing the batch domain, migration, API contract, tenant
isolation, idempotency, and state-machine behavior.
