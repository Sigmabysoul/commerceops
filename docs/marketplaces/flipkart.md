# Flipkart adapter

The Phase 3 adapter accepts PDF uploads through `POST /api/v1/flipkart/jobs` and returns a persisted background job. Files are limited to 20 MiB, identified by server-generated UUIDs, stored outside PostgreSQL under `FILE_STORAGE_DIR`, and deduplicated per company by SHA-256.

## Supported document form

Parser version `flipkart-pdf-v1` supports text-based PDFs whose content streams contain literal PDF text operators (`Tj`/`TJ`), including Flate-compressed streams. It recognizes labels containing the Flipkart marker and labeled AWB/tracking, order ID, SKU/FSN/seller SKU, and quantity fields.

Scanned/image-only PDFs, custom font encodings that do not expose literal text, encrypted PDFs, OCR, and coordinate-based multi-label splitting are not supported. They fail safely or enter manual review; they are never assigned invented identifiers or quantities.

## Processing

Two bounded in-process workers consume a 32-job queue. Jobs move through `queued`, `processing`, then `processed`, `needs_review`, or `failed`. Queued/interrupted jobs are recovered at startup. Every order records its source file, page, and parser version.

Exact active Flipkart SKU mappings in Product Master resolve products. Missing mappings remain unresolved. Missing quantities remain SQL `NULL` with `quantity_source=missing`; the adapter never assumes one.

AWB and order IDs are unique per company and marketplace for authoritative records. Repeated source hashes return the existing job; repeated identifiers in another file create visible duplicate review records.
