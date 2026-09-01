# Meesho adapter — Phase 10

The isolated Meesho adapter lives under `internal/marketplace/meesho`. It
receives bounded, real page-numbered text from the shared Poppler extractor and
returns normalized records containing only explicitly detected:

- Meesho sub-order ID, preferred over an order ID when present
- labeled AWB or tracking identifier
- labeled supplier/seller SKU
- positive explicitly labeled quantity

Parser version `meesho-labeled-v1` never reads storage, queries Product Master,
persists records, performs authorization, or changes inventory. Generic
marketplace orchestration owns those responsibilities.

## Recognition and review behavior

A page must contain an order/sub-order marker, an AWB/tracking marker, and at
least three independent Meesho label signals. A brand mention alone is not a
shipping label. Each accepted record persists its source file, real one-based
source page, extraction method, parser version, processing job, and normalized
order/item relationship.

Only a unique value at a supported labeled field is accepted. Multiple
different values for the same field are ambiguous and remain empty. A missing,
zero, malformed, or conflicting quantity remains SQL `NULL` with
`quantity_source=missing`; it never defaults to one. Unknown Seller SKUs remain
unresolved until an exact active `meesho` Product Master mapping exists. Missing
or ambiguous order, AWB, SKU, quantity, and duplicate business identifiers are
persisted as review-required rather than guessed.

## Shared infrastructure

Meesho uses the existing authenticated multipart upload, 20 MiB PDF limit,
server-generated tenant object key, company/marketplace SHA-256 deduplication,
PostgreSQL job/lease worker, retry, normalized marketplace tables, source-page
document relation, permissions, entitlement, audit, and typed frontend result
view. A Flipkart or Amazon worker cannot claim a Meesho job.

## Batch, printing, inventory, reporting, and returns

Resolved Meesho orders use the existing batch system, configurable fallback and
exact-product worker assignments, Product Master totals, readiness rules, and
source traceability. `source-page-v1` preserves each complete source label page
without cropping or overlays. Sorting remains deterministic through the shared
batch rules. Invoice export is rejected because no deterministic invoice
association is established. Print and reprint jobs remain fully traceable and
inventory-neutral.

Only explicit ready-batch outbound confirmation crosses the central Inventory
boundary. It aggregates canonical products, creates one immutable
`ecommerce_out` entry per product, and is idempotent per company/batch. The
shared dashboard derives Meesho order, batch, print, product, outbound, return,
and cancellation metrics through its marketplace filter. Company-wide stock-in,
general adjustment, correction, consignment movement, and current-balance
semantics remain unchanged. The generic Returns domain accepts resolved Meesho
orders and explicit normalized quantities.

## Evidence-based limits

Representative private Meesho PDFs were not supplied for Phase 10. The adapter
therefore enables text extraction only and does not infer cross-page
relationships or require OCR. The repository contains a sanitized structural
text fixture; maintainers can validate an authorized private PDF without
committing or logging it by setting `MEESHO_PRIVATE_SAMPLE`.

No Meesho-specific page association, OCR, crop geometry, invoice output, or
enrichment has been invented. Future changes to those behaviors require
representative evidence and new sanitized regressions. Upload, processing,
printing, and reprinting remain inventory-neutral.
