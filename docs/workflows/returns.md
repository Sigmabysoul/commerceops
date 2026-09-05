# Returns and cancellations workflow

Marketplace cancellation and physical-return processing share normalized
Flipkart/Amazon/Meesho/Myntra/Snapdeal orders and canonical Product Master items. Myntra
CSV rows remain ineligible until explicit quantity evidence exists.

```text
Cancellation recorded
→ pre-outbound dispatch prevention OR post-outbound evidence retained
→ close (inventory-neutral)

Expected return
→ explicit physical receipt
→ explicit inspection disposition
→ restock through Inventory OR terminal non-restockable state
→ optional compensating correction
→ close (inventory-neutral)
```

Cancellation alone never restores stock. Receipt and inspection alone never
restore stock. Only positive, explicitly `restockable` quantities cross the
central Inventory boundary. Damaged, rejected, wrong-product, and missing lines
remain inventory-neutral. Incorrect restocks use negative compensating ledger
entries; no event or stock transaction is rewritten.

Every action is tenant-scoped and permission-checked. Request hashes and
idempotency keys make retries exact. Case/item locks bound partial and concurrent
quantities, while normalized-order locks serialize pre-outbound cancellation
against batch readiness and outbound confirmation.
