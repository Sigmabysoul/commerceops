# Meesho adapter — Phase 10 Batch A

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

## Batch A limits

Representative private Meesho PDFs were not supplied for Batch A. The adapter
therefore enables text extraction only and does not infer cross-page
relationships or require OCR. The repository contains a sanitized structural
text fixture; maintainers can validate an authorized private PDF without
committing or logging it by setting `MEESHO_PRIVATE_SAMPLE`.

Batch/printing participation, any evidence-based Meesho page association or
print geometry, outbound inventory confirmation, reporting, and returns remain
later Phase 10 batches. Upload and processing are inventory-neutral.
