# Myntra packed-orders adapter — Phase 11 Batch A

The isolated adapter under `internal/marketplace/myntra` accepts only the
evidence-backed UTF-8 packed-orders CSV form. It does not parse or generate PDF
labels.

## Normalized identifiers

- `Order id` → shared `marketplace_order_id`
- `Tracking_id` → shared `awb`
- `Seller_sku_code` → item `raw_sku` and exact Product Master mapping key
- CSV record number → persisted source position (`source_page` in the current
  shared contract, exposed to the Myntra UI as a row)
- `Myntra SKU code`, `Store Packet ID`, `Order_release_id`, `Status`, `Packed
  On`, and `Created On` → immutable extraction evidence metadata

The adapter validates required headers, UTF-8 encoding, CSV structure, and
observed timestamp shapes. Duplicate order or tracking identifiers inside the
same file become explicit review warnings. Generic tenant/marketplace source
hashing protects exact duplicate files, while a required upload idempotency key
returns exact replays and conflicts on changed content.

## Product and quantity safety

Only an exact active `myntra` mapping for `Seller_sku_code` may resolve a
canonical product. Products are never auto-created. The available production
CSV contains no proven quantity field, so the adapter never emits a quantity
and the normalized item always records `quantity_source=missing`. Consequently
every imported order needs review and the shared batch readiness, Inventory
outbound, and Returns quantity boundaries remain closed.

## Deferred PDF behavior

Shipping-label classification, invoice association, OCR, coordinates, crop
geometry, label dimensions, barcode-safe overlays, and PDF quantity evidence
are deferred until representative production Myntra labels are supplied. No
print generator is registered for Myntra in Batch A.
