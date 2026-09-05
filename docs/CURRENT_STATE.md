## Current Phase

Phase 14 — Printing Automation

## Current Branch

`phase/14-printing-automation`

## Approved Baseline

Phase 13 baseline:

`41ae0191f7b4b3a807995b8d8cb064772e34e568`

## Phase Status

`PHASE_14_DESIGN_COMPLETE_IMPLEMENTATION_PENDING`

Phases 0–10 are implemented and Phase 10 is the approved starting baseline.
The owner explicitly authorized Phase 11 Myntra Batch A only.

The owner explicitly authorized Phase 12 Snapdeal from the remediated Phase 11
baseline.

The owner explicitly authorized Phase 13 after Phase 12 completion.

The owner explicitly authorized Phase 14 after Phase 13 completion.

## Phase 14 Start

- [x] dedicated Phase 14 branch
- [x] bounded trigger set confirmed from the approved phase specification
- [x] schema, timezone/DST, scheduler, event, safety, UX, reporting, and test design
- [ ] migration and Automation domain implementation
- [ ] authoritative Batch and Consignment event integration
- [ ] scheduler lifecycle and API wiring
- [ ] responsive Automation workspace and derived reporting
- [ ] complete PostgreSQL-backed verification

## Phase 13 Delivery

- [x] pre-code schema, protocol, threat model, platform, UX, API, and test design
- [x] tenant printer-agent registry with hashed one-time revocable credentials
- [x] friendly registered printers with heartbeat presence and enable controls
- [x] canonical physical print jobs with semantic idempotency and append-only events
- [x] atomic agent claims, guarded downloads, lease reporting, and explicit retry
- [x] durable local journal preventing reconnect duplicate submission
- [x] fixed-argument Linux/CUPS backend behind a future Windows interface
- [x] validated tenant Print Library PDFs using server-owned object-storage keys
- [x] mobile-first installable Quick Print UI with favorites, categories, search,
  recent history, default printer/copies, bounds, and large-copy confirmation
- [x] existing marketplace artifacts enter the same physical queue
- [x] dedicated permissions, tenant isolation, audits, concurrency coverage, and
  verified Inventory neutrality
- [x] no scheduled or automatic job creation introduced

## Phase 13 Verification

Phase 13 passed `make verify-full` against a fresh disposable PostgreSQL 18
database migrated through `000021`. PostgreSQL-backed coverage includes tenant
and permission isolation, credential hashing/revocation, printer presence,
validated library PDFs, metadata updates, quick-print copy guardrails,
idempotency conflicts/replays, concurrent claims, hash-authorized downloads,
state reporting, lease expiry, explicit retry, existing batch artifacts,
append-only events/audit, reconnect journaling, migration up/down, and Inventory
neutrality. Go vet/tests, server and agent builds, frontend typecheck/lint/build,
OpenAPI parsing, and repository checks passed.

## Phase 12 Delivery

- [x] isolated `snapdeal-packslip-v1` text adapter with shipping/invoice classification
- [x] exact SUBORDER association without positional guessing
- [x] invoice SKU CODE Product Master resolution and compact shipping-code evidence
- [x] explicit cross-page quantity agreement with conflict/review behavior
- [x] shared secure upload, leases, duplicate protection, traceability, authorization, and audit
- [x] shared batch, configurable assignment, sorting, artifacts, invoice export, and reprints
- [x] measured `snapdeal-packslip-enriched-v1` output preserving the full shipping page
- [x] central Inventory outbound, Returns/cancellations, and marketplace reporting reuse
- [x] typed frontend processing, batch, reporting, and Returns selectors
- [x] migration `000020` adds only the Snapdeal claim index
- [x] no Snapdeal-specific order, batch, inventory, return, or reporting tables

## Phase 12 Evidence Limit

One private two-page production PDF establishes the supported baseline only.
The courier barcode value is not in its extractable text layer; the print path
preserves the barcode image, while normalized AWB remains absent rather than
guessed. Other layouts require new evidence and sanitized regressions.

## Phase 11 Batch A Delivery

- [x] bounded UTF-8 Myntra packed-orders CSV import through shared source/job orchestration
- [x] `Order id` and `Tracking_id` normalized as order/AWB identifiers
- [x] `Seller_sku_code` used for exact Myntra Product Master mapping
- [x] Myntra SKU, packet/release IDs, status, and timestamps preserved as evidence metadata
- [x] missing quantity retained without a default; all current Myntra rows remain review-required
- [x] tenant/module/permission enforcement, source deduplication, upload idempotency, leases, retry, and audit reuse
- [x] batch/reporting/returns selectors recognize Myntra while readiness and quantity-dependent workflows remain blocked
- [x] no Myntra-specific order, inventory, returns, batch, reporting, or printing tables
- [x] no PDF parser, cropper, invoice association, OCR, overlay, or print generator
- [x] migration `000019` adds generic upload idempotency evidence and the Myntra claim index

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

## Phase 11 Evidence Limit

No representative Myntra label PDF was supplied. Batch A therefore implements
only the real packed-orders CSV contract. Quantity and all PDF layout,
association, extraction, and enrichment behavior remain deferred.

## Next Allowed Task

Implement and verify only the approved Phase 14 Printing Automation scope.
