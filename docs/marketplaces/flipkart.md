# Flipkart adapter

The Phase 3 adapter accepts PDF uploads through `POST /api/v1/flipkart/jobs` and returns a persisted background job. Files are limited to 20 MiB, identified by server-generated UUIDs, stored through the platform object-storage interface, and deduplicated per company by SHA-256. Development defaults to the local implementation under `FILE_STORAGE_DIR`; production can select the S3-compatible implementation without changing marketplace code. PostgreSQL metadata owns tenant association, and storage keys are generated server-side with the authenticated company ID as their prefix.

## Supported document form

Parser version `flipkart-text-v2` receives actual page-delimited UTF-8 text from the platform PDF extractor. The current extractor uses Poppler `pdfinfo` and `pdftotext`; the API does not interpret raw PDF streams. It recognizes pages containing the Flipkart marker and labeled AWB/tracking, order ID, SKU/FSN/seller SKU, and quantity fields. Persisted `source_page` is the real one-based PDF page number.

Scanned/image-only PDFs, encrypted PDFs, OCR, and coordinate-based multi-label splitting are not supported. They fail explicitly; identifiers and quantities are never invented. Extraction is bounded to 100 pages, 1 MiB of text per page, 10 MiB per document, and a 30-second subprocess timeout.

## Processing

Two bounded in-process workers claim durable Flipkart jobs from PostgreSQL with `FOR UPDATE SKIP LOCKED`. An in-memory signal only reduces polling latency and is not the source of truth, so saturation cannot fail a persisted job. Jobs move through `queued`, `processing`, then `processed`, `needs_review`, or `failed`. Unexpected worker errors are recorded and transition claimed jobs to `failed`.

Startup recovery changes only `marketplace_key='flipkart'` processing jobs back to queued. Phase 3 assumes one active API instance because processing jobs do not yet have owner/lease columns; multiple instances can safely claim queued jobs, but one instance restarting could requeue a job actively owned by another instance. A lease is required before multi-instance deployment.

Exact active Flipkart SKU mappings in Product Master resolve products. Missing mappings remain unresolved. Missing quantities remain SQL `NULL` with `quantity_source=missing`; the adapter never assumes one.

AWB and order IDs are unique per company and marketplace for authoritative records. The source hash unique constraint is authoritative and concurrent conflicts return the existing tenant job. Repeated identifiers in another file create visible duplicate review records.
