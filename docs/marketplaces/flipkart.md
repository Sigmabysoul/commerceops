# Flipkart adapter

The Phase 3 adapter accepts PDF uploads through `POST /api/v1/flipkart/jobs` and returns a persisted background job. Files are limited to 20 MiB, identified by server-generated UUIDs, stored through the platform object-storage interface, and deduplicated per company by SHA-256. Development defaults to the local implementation under `FILE_STORAGE_DIR`; production can select the S3-compatible implementation without changing marketplace code. PostgreSQL metadata owns tenant association, and storage keys are generated server-side with the authenticated company ID as their prefix.

## Supported document forms

Parser version `flipkart-text-v3` receives actual page-delimited UTF-8 text from the platform PDF extractor. The current extractor uses Poppler `pdfinfo` and `pdftotext -layout`; the API does not interpret raw PDF streams. Persisted `source_page` is always the real one-based PDF page number supplied by the extractor.

The adapter supports both the original labeled-field fixtures and the validated modern Flipkart layout. The modern layout has one shipping label and one tax invoice on each physical A4 page. Representative validation covered nine production PDFs totaling 84 pages and nine corresponding CropBox PDFs totaling another 84 pages. Observed courier variations use the same label structure with FMPP-, FMPC-, SF-prefixed, or numeric AWBs.

Modern pages are recognized from multiple shipping-label signals rather than the word `flipkart`: an AWB/tracking marker, a structured `OD...` order identifier, the `SKU ID | Description | QTY` table header, a shipping/customer address marker, and branding when it is extractable. At least three signals are required. A standalone invoice or unrelated document that merely mentions Flipkart is not authoritative.

## Shipping-label extraction rules

Modern Poppler output places the authoritative shipping-label text before a line beginning `Tax Invoice`. The parser limits field extraction to that region, so invoice order, invoice number, SKU, and quantity text cannot override shipping-label values. Structured `OD...` identifiers occur in the label region even when they do not have a nearby `Order ID:` caption.

- **AWB:** accepted only from an anchored `AWB`, `AWB No.`, or tracking field line. The value remains restricted to a bounded alphanumeric/hyphen token, so invoice numbers elsewhere are not treated as AWBs.
- **Order ID:** accepted only as a bounded Flipkart `OD...` identifier from the shipping-label region. Generic words such as `Invoice` are never order IDs.
- **SKU:** for modern labels, the SKU comes from the first product row following `SKU ID | Description | QTY`; the header word `ID` is not a value. Validated SKUs include mixed case and the punctuation used by the representative files. Explicit legacy `SKU:`, `FSN:`, and `Seller SKU:` fields remain supported.
- **Quantity:** for modern labels, a positive integer is accepted only when Poppler places it under the header's `QTY` column. A number at the end of a description is not a quantity. Explicit legacy `Qty:` and `Quantity:` fields remain supported. Missing, zero, malformed, or ambiguous quantities remain missing and force review; they never default to one.

CropBox samples retain the original A4 MediaBox and content streams. Poppler therefore still returns invoice text outside the visible crop. The same shipping/invoice boundary is applied to full-page and cropped inputs; Phase 3 does not generate cropped output.

Scanned/image-only PDFs, encrypted PDFs, OCR, and coordinate-based multi-label splitting are not supported. They fail explicitly; identifiers and quantities are never invented. Extraction is bounded to 100 pages, 1 MiB of text per page, 10 MiB per document, and a 30-second subprocess timeout.

Multi-product tables and layouts without enough validated label signals are not treated as fully supported. If a table row cannot be interpreted unambiguously within the existing normalized single-item boundary, SKU and/or quantity remain missing for manual review rather than selecting a guessed value. No OCR is required for the representative modern text PDFs.

Sanitized A4 and CropBox regression PDFs live under `services/api/internal/marketplace/flipkart/testdata`. Private production files remain outside the repository. Maintainers with authorized local samples can run the same Poppler/parser validation by setting `FLIPKART_PRIVATE_SAMPLES_DIR`; the test only selects `invoice_labels_*.pdf` and `flipkart_cropped*.pdf` and never logs extracted identifiers.

## Processing

Two bounded in-process workers claim durable Flipkart jobs from PostgreSQL with `FOR UPDATE SKIP LOCKED`. Each API process has a cryptographically random, non-secret worker ID. A claim atomically changes queued work to `processing`, or reclaims processing work only after its lease expires, and establishes a two-minute lease. A heartbeat renews the lease every 30 seconds while PDF work runs. Completion and failure verify current ownership and clear the lease in the same transaction as the final state.

Startup recovery is read-only: it signals local workers when queued or expired Flipkart work exists and never rewrites healthy `processing` jobs. PostgreSQL remains the job authority; the in-memory signal only reduces polling latency. A stale instance cannot commit authoritative orders or failure state after another worker has reclaimed its expired lease. Non-Flipkart jobs are excluded from recovery and claim queries.

Exact active Flipkart SKU mappings in Product Master resolve products. Missing mappings remain unresolved. Missing quantities remain SQL `NULL` with `quantity_source=missing`; the adapter never assumes one.

AWB and order IDs are unique per company and marketplace for authoritative records. The source hash unique constraint is authoritative and concurrent conflicts return the existing tenant job. Repeated identifiers in another file create visible duplicate review records.
