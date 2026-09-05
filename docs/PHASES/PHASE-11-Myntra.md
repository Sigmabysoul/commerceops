Implement CommerceOps Phase 11 — Myntra Marketplace Processing.

Do not begin until Phase 10 is approved.

Follow the existing shared marketplace architecture.

Myntra must be implemented as an isolated adapter and must NOT duplicate generic
marketplace, storage, Product Master, batch, Inventory, reporting, returns, or
printing infrastructure.

Use representative private production Myntra PDFs.

Determine actual production document structure before committing parser rules.

Support extraction of stable production identifiers such as:
- marketplace order/item ID
- tracking/AWB
- seller SKU
- explicit quantity
- invoice/reference information where required

Rules:
- no guessed quantity
- no auto-created Product Master products
- unknown/ambiguous values → review
- deterministic association using stable identifiers
- duplicate files/orders visible and idempotent
- source/page/parser traceability

Integrate normalized Myntra orders with:
- Product Master
- Batch + Printing
- central Inventory outbound
- Dashboard + Reporting
- Returns

If Myntra requires special label manipulation, place it inside:
internal/marketplace/myntra

Do not introduce a second print-job system.

Require:
- Myntra module entitlement
- tenant context
- label permissions

Keep real PDFs private and use sanitized regression fixtures.

Run:
- parser tests
- PostgreSQL integration tests
- duplicate/idempotency tests
- tenant/auth tests
- batch/print tests
- inventory tests
- reporting isolation tests
- Returns compatibility
- frontend verification
- full CI-equivalent verification

Update CURRENT_STATE.md and docs.

STOP.
Do not begin Snapdeal / Phase 12.