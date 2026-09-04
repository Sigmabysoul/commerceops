# Snapdeal adapter — Phase 12

Parser version `snapdeal-packslip-v1` supports the representative two-page,
297.637×419.512 pt Snapdeal document baseline. Shipping pages require delivery,
suborder, shipped-from, and Snapdeal-reference signals. Invoice pages require
tax-invoice, invoice-number, SKU-code, suborder, and HSN signals.

`SUBORDER` is the authoritative order and association key. One shipping page
and one invoice page with the same unique suborder form a high-confidence
record. Missing, mismatched, or repeated roles are never position-associated.
Invoice `SKU CODE` is the raw Product Master key; the compact shipping code is
preserved only as evidence. Quantity must be positive and unique across both
pages, otherwise the record needs review.

Generation version `snapdeal-packslip-enriched-v1` validates the measured page
geometry, raster-preserves the complete source at 200 DPI, and places the
SKU/QTY banner in the observed blank lower band (PDF y=55–97 pt), below the
main label and above its footer page number. Invoice export uses the persisted
exact-suborder invoice page. Printing and reprinting are inventory-neutral.

The representative private PDF remains outside Git. Set
`SNAPDEAL_PRIVATE_SAMPLE` to run its structural and print regression. Its
courier barcode text is graphical rather than part of the extractable PDF text;
the generator preserves it visually, while the parser leaves AWB empty instead
of inventing a value. Other Snapdeal layouts remain unsupported until sampled.
