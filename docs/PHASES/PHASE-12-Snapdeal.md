Implement CommerceOps Phase 12 — Snapdeal Marketplace Processing.

Start only from the formally approved Phase 11 baseline.

Reuse the marketplace infrastructure built for Flipkart, Amazon, Meesho and
Myntra.

Create an isolated Snapdeal adapter.

Do NOT duplicate:
upload/storage/jobs/workers/Product Master/batches/Inventory/reporting/returns.

First inspect representative private Snapdeal production PDFs and document the
actual stable identifiers and page structure.

Implement:
- deterministic extraction
- explicit quantity
- AWB/tracking
- marketplace order identifiers
- raw seller SKU
- Product Master resolution
- review state for ambiguity
- duplicate source/business identifier protection
- parser/source traceability
- normalized batch compatibility
- optional Snapdeal print adapter only if required
- central ecommerce outbound inventory
- reporting inclusion
- returns compatibility

No stock movement during:
upload
parse
batch creation
PDF generation
printing
reprinting

Only approved outbound confirmation moves inventory.

Require entitlement and permission enforcement.

Create sanitized fixtures and full PostgreSQL regressions.

Update docs and CURRENT_STATE.

STOP before Phase 13.