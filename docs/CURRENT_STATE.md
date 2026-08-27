# CommerceOps Current State

Version: 0.4.0-dev

Current Phase:
Phase 3 — Flipkart Processing

Status:
Phase 3 implementation in progress

Implemented in current working tree:
- Tenant-owned PDF upload with a 20 MiB limit and SHA-256 source deduplication
- Local-development file storage configured by `FILE_STORAGE_DIR`, behind the marketplace service boundary
- Persisted jobs with a bounded two-worker in-process queue and restart recovery
- Isolated `flipkart-pdf-v1` text parser
- AWB, order ID, SKU, and explicit quantity extraction
- Exact Product Master resolution and unresolved review state
- Tenant-scoped duplicate identifier protection
- Page-level warnings/errors and normalized results API
- Flipkart upload, progress, results, and review UI
- Backend entitlement/permission checks and upload/completion audit events

Remaining review work:
- PostgreSQL-backed Phase 3 integration and migration verification
- Sanitized representative PDFs for every production Flipkart document shape
- External architecture review

Approved:
- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master

Phase 2:
Completed and externally approved.

Current Goal:
Implement the first marketplace adapter for Flipkart.

Phase 3 Scope:
- Secure Flipkart PDF upload
- Source file storage and tenant ownership
- Background processing jobs
- Flipkart document/page/label inspection
- AWB and order identifier extraction
- Raw marketplace SKU extraction
- Reliable quantity extraction where available
- Product Master resolution
- Duplicate source-file detection
- Duplicate AWB/order detection
- Explicit processing states
- Manual review for unresolved or invalid records
- Traceable processing errors and warnings
- Functional Flipkart upload/results UI
- Permission and Flipkart module-entitlement enforcement
- Audit events for important user/business actions
- Sanitized Flipkart test fixtures

Important Invariants:
- Flipkart-specific parsing logic remains isolated in the Flipkart adapter.
- company_id comes from authenticated server context.
- Product identity comes from Product Master, not marketplace SKU strings.
- Unknown SKUs are never silently mapped.
- Quantity extraction failures never silently become trusted quantity = 1.
- Duplicate uploads/orders never create duplicate authoritative records silently.
- No inventory movement occurs in Phase 3.
- No final printing/batch orchestration occurs in Phase 3.
- Processing must be safely retryable.
- Every normalized result must remain traceable to its source file/page and parser version.

Not Implemented:
- Final batch orchestration
- Printing workflow
- Inventory deductions
- Returns/cancellations
- Consignment
- Amazon
- Meesho
- Myntra
- Snapdeal
- Printer agent

Review Gate:
Phase 3 must pass:
- PostgreSQL migration verification
- backend tests
- fixture-based Flipkart parser tests
- tenant-isolation tests
- duplicate/idempotency tests
- background-job state tests
- backend CI
- frontend CI
- external architecture review

Next Phase:
Phase 4 — Batch + Printing

Do not begin Phase 4 until Phase 3 has passed external review.
