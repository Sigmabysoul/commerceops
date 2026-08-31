# Amazon adapter — Phase 7 Batch B

The isolated Amazon adapter lives under `internal/marketplace/amazon`. It
receives bounded, real page-numbered text from the shared Poppler/Tesseract
extractor and returns normalized records containing only explicitly detected:

- Amazon order ID in the `NNN-NNNNNNN-NNNNNNN` form
- labeled AWB/tracking identifier
- labeled seller/merchant SKU, or seller SKU in a validated Amazon invoice row
- positive explicit quantity

Parser version `amazon-associated-v3` never reads storage, queries Product Master,
persists records, performs authorization, or changes inventory. Generic
marketplace orchestration owns those responsibilities.

## Association and traceability

Text-empty pages are rendered at 300 DPI and OCRed only for Amazon. Every page
retains its real one-based page number and extraction method. The adapter
classifies shipping labels and invoices and groups them first by an exact
canonical Amazon order ID. A one-page adjacency fallback is accepted only when
one side lacks an order ID, the opposite roles are mutually unique candidates,
the identified side has one stable order ID, and the combined pair has a unique
AWB, invoice SKU, and explicit quantity. Conflicting identifiers and competing
neighbors are never position-matched.

The canonical order points at the shipping-label page when one is unique.
`marketplace_order_documents` preserves both the label and invoice source pages,
their roles, source file, and extraction methods for later print work.

## Supported structures

The adapter recognizes text-extractable Amazon label pages with multiple Amazon
signals and text-extractable tax-invoice pages with stable order identifiers.
Validated invoice rows use this precedence: bracketed SKU immediately before
`HSN`, a bracketed `_`/`-` code, a token immediately before `HSN`, then
validated labeled/ASIN invoice fallbacks. Quantity is accepted only from a
labeled field or an unambiguous invoice price/quantity/amount sequence. ASIN is
not treated as seller SKU.

The private production sample used for validation contains ten A4 pages: five
image-only shipping labels alternating with five text invoices. Bounded OCR
extracts the label identifiers, text extraction supplies invoice fields, and
all five pairs associate by exact order ID without persisting or logging the
private document.

## Review behavior and limits

Unknown Amazon SKUs are resolved only through exact active Amazon Product
Master mappings. Missing or ambiguous SKU, quantity, order ID, or AWB becomes a
review warning; missing quantity never defaults to one. Duplicate source hashes
and business identifiers use the shared tenant/marketplace constraints.

## Batch and printable output

Resolved Amazon orders use the shared batch, worker-assignment, print-job,
artifact, download, and reprint workflow. The Amazon print adapter accepts only
validated A4 pages and explicit SKU/quantity. It renders the entire original
shipping page, scales it uniformly into a reserved A4 content region, and adds
a large readable enrichment banner without cropping barcodes, QR codes,
addresses, AWB, or routing information. Optional invoice output uses the
persisted associated invoice page. Generation version is
`amazon-a4-enriched-v1`; printing and reprinting remain inventory-neutral.

Inventory integration changes and reporting verification are deferred beyond
Batch B. The private production PDF is not committed. The repository contains
a sanitized two-page PDF plus sanitized association, extraction, ambiguity, and
print regression cases.
