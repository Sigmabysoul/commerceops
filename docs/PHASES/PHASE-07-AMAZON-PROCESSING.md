Implement CommerceOps Phase 7 — Amazon marketplace processing.

Do not begin until Phase 6 is approved.

Amazon must use the existing CommerceOps marketplace architecture established by Flipkart.

Do not duplicate generic upload/job/storage/batch/inventory infrastructure.

## Goal

Support production Amazon label/invoice documents and normalize them into the same canonical CommerceOps marketplace order model.

## Adapter boundary

Create an isolated Amazon adapter.

Amazon-specific parsing belongs only inside the Amazon marketplace module.

Generic functionality must continue to live in shared platform/domain modules.

## Production samples

Use representative real Amazon PDFs privately.

Do not commit customer/private production documents.

Create sanitized regression fixtures preserving necessary document structure.

## Extraction

Support the real Amazon workflow represented by production samples.

Expected concepts may include:

* Amazon order ID
* AWB/tracking identifier
* seller SKU
* quantity
* shipping-label page/region
* invoice/description information where required

Do not assume Amazon's layout matches Flipkart.

Do not silently default missing quantities to 1.

## Invoice/shipping association

Where Amazon shipping labels and invoices contain complementary information, establish deterministic association using stable document identifiers such as the actual Amazon order number.

Do not associate pages based merely on page position when a stronger identifier exists.

Ambiguous associations must enter review.

## Product Master

Amazon raw SKU
→ Amazon SKU mapping
→ canonical Product Master product

Unknown SKU must remain unresolved.

Do not create products automatically from raw marketplace strings unless using the explicit Product Master training workflow.

## Duplicate handling

Prevent duplicate authoritative orders using appropriate Amazon business identifiers.

Repeated source files and repeated marketplace identifiers must be visible and idempotent.

## Batch and printing integration

Amazon normalized orders must become eligible for existing batch/printing infrastructure without creating Amazon-only batch infrastructure.

If Amazon output requires specialized label manipulation, keep that inside the Amazon print adapter while using shared artifact/job concepts.

## Inventory

Amazon outbound must use centralized Inventory.

Do not deduct stock during parsing, PDF generation or reprint.

Use the same approved outbound confirmation event.

## Dashboard

Phase 6 reporting should naturally include Amazon after entitlement is enabled.

Do not build a separate Amazon dashboard system.

## Permissions / entitlements

Require:

* Amazon module entitlement
* appropriate label-processing permissions
* tenant context

## Tests

Cover:

* real/sanitized Amazon parser layouts
* source-page traceability
* order/invoice association
* missing quantities
* unknown SKU
* duplicate files/orders
* tenant isolation
* Product Master resolution
* batch compatibility
* inventory integration
* dashboard reporting inclusion
* retries/idempotency
* full PostgreSQL verification

Update docs and `CURRENT_STATE.md`.

STOP.

Do not begin Phase 8.
