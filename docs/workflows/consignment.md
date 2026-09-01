# Consignment workflow

Consignments are company-scoped SO/order requirements linked only to canonical
Product Master products. Manual sources may omit a source reference; imported
sources must record one. Dealer/customer and pouch/file references remain
traceability fields. `order_reference` is company-unique; `pouch_reference` is
not globally unique and must not be used as record identity.

The state path is:

`created → allocated → picking → ready → packing → packed → outbound → completed`

`created` through `packed` may instead become `cancelled`. Invalid skips,
backward transitions, post-outbound cancellation, incomplete ready/packed
transitions, and stale expected versions are rejected.

Allocation reserves the sum of required quantities per Product Master product.
Workers update absolute `ready_quantity` and `packed_quantity` values on their
assigned department lines, with `0 ≤ packed ≤ ready ≤ required`. Different
lines have independent versions so workers can progress concurrently; a stale
update to the same line conflicts. Department-scoped readers receive only their
assigned lines and no broad event notes.

Outbound confirmation is allowed only after every line is fully packed. In the
same transaction Inventory deducts On-hand, closes Reserved, writes one
immutable `consignment_out` transaction per product, and closes each source
reservation with a consumption reason. Exact replay returns the original
result; a changed payload conflicts. Cancellation closes active reservations
without a ledger deduction.
