# Amazon adapter — Phase 7 Batch A

The isolated Amazon adapter lives under `internal/marketplace/amazon`. It
receives bounded, real page-numbered text from the shared Poppler extractor and
returns candidate normalized records containing only explicitly detected:

- Amazon order ID in the `NNN-NNNNNNN-NNNNNNN` form
- labeled AWB/tracking identifier
- labeled seller/merchant SKU, or seller SKU in a validated Amazon invoice row
- positive explicit quantity

Parser version `amazon-text-v1` never reads storage, queries Product Master,
persists records, performs authorization, or changes inventory. Generic
marketplace orchestration owns those responsibilities.

## Supported Batch A structures

Batch A recognizes text-extractable Amazon label pages with multiple Amazon
signals and text-extractable tax-invoice pages with stable order identifiers.
Validated invoice rows may place the ASIN and seller SKU on one line or split
the seller SKU onto the following line. Quantity is accepted only from a
labeled field or an unambiguous invoice price/quantity/amount sequence. ASIN is
not treated as seller SKU.

The private production sample used for validation contains ten A4 pages:
five image-only shipping labels alternating with five text invoices. The
invoices yield explicit order ID, seller SKU, and quantity on their actual
source pages. The image-only labels yield no Poppler text, so their AWBs are not
invented and they are not position-matched to invoices in Batch A.

## Review behavior and limits

Unknown Amazon SKUs are resolved only through exact active Amazon Product
Master mappings. Missing or ambiguous SKU, quantity, order ID, or AWB becomes a
review warning; missing quantity never defaults to one. Duplicate source hashes
and business identifiers use the shared tenant/marketplace constraints.

OCR, final shipping-label/invoice association, Amazon print cropping/output,
batch compatibility changes, and inventory changes are deferred beyond Batch A.
The private production PDF is not committed. The repository contains a
sanitized two-page text PDF preserving the supported parser structure.
