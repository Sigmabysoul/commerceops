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

Batch B will add bounded PDF generation and print-artifact persistence. Batch C
will add the frontend workflow. Neither behavior is part of Batch A.
