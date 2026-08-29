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
- batch schema/state model
- tenant-scoped batch APIs
- selection of eligible processed Flipkart records
- permissions/audit/idempotency foundations
- tests

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

Phase 4 implementation has not yet been completed.

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

These Phase 3 results are historical baseline evidence and must not be presented as verification of future Phase 4 changes.

## Next Allowed Task

Implement Phase 4 Batch A:

**Batch domain and persistence foundation only.**

Before editing, read:

- `AGENTS.md`
- `docs/AI_WORKFLOW.md`
- `docs/MASTER_SPEC.md`
- `docs/ARCHITECTURE.md`
- `docs/DOMAIN_RULES.md`
- `docs/PHASES/PHASE-04-BATCH-PRINTING.md`
- this file

Plan first.

Do not implement PDF output generation, sorting UI, invoice export UI, inventory, other marketplaces, or Phase 5 in the same task unless the active Phase 4 specification explicitly requires them for the Batch A foundation.

Implement the smallest coherent Batch A change, test it, commit it, provide the required completion report, and STOP.