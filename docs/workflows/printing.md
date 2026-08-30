# Batch and printing workflow

Phase 4 Batch A establishes the batch domain only. It does not generate or send
print output.

A draft batch belongs to the authenticated company and one marketplace. Its
ordered members reference normalized Phase 3 marketplace orders, retaining the
source file, processing job, source page, marketplace order ID, and AWB through
those relationships. Product totals are calculated from resolved Product Master
IDs and explicit normalized quantities rather than copied counters or raw SKU
text.

## Batch A state machine

```text
draft → ready
draft → cancelled
```

Repeated requests for the current target state are safe no-ops. Other
transitions are rejected. A draft cannot become ready while any member order is
unresolved, lacks an item, lacks a Product Master ID, or lacks an explicit
positive quantity. Cancellation and readiness transitions are audited.

Batch creation requires an idempotency key scoped to the company. Reusing the
key with the identical marketplace and ordered member list returns the existing
batch; reusing it with different input is rejected. A normalized order may be a
member of only one batch in this foundation, preventing ambiguous operational
ownership.

## Batch B output generation

A ready batch can generate a print job using the validated modern Flipkart A4
profile. Poppler/Cairo renders the shipping-label region at `(188,26)` with a
normalized `218 × 360 pt` page. The invoice begins below that label boundary and
is rendered separately as `595 × 456 pt` only when invoice export is enabled.
Rendering uses the original MediaBox even for source files whose CropBox already
hides the invoice, then creates new normalized pages; hidden invoice content is
not retained in label output.

`Sort Labels` off preserves batch position. When enabled, deterministic ordering
uses Product Master internal code, normalized SKU, marketplace order ID, then
batch position. Invoice pages follow the exact generated label order.

Generated PDFs are stored through the platform object-storage boundary. Print
job items snapshot source file, processing job, normalized order, source page,
and output position. Artifact metadata records kind, hash, size, page count, and
generation version `flipkart-a4-v1`. Exact idempotent replays return the existing
job; failures are persisted without creating an artifact or inventory effect.

Supported output is intentionally limited to the production-validated A4
single-item layout. Unknown geometry, scanned PDFs, and other marketplace forms
fail instead of using guessed crop coordinates. Generation is limited to 500
pages, 20 MiB per source, 100 MiB per artifact, and 60 seconds.

Batch C will add the frontend workflow. Physical printing, reprinting, worker
assignment, and inventory remain unimplemented.
