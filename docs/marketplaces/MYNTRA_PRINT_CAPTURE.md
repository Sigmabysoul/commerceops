# Myntra print-payload capture

Use this procedure to collect the source evidence required to resume Phase 19. Keep original
customer and seller data private; no original capture belongs in Git.

## Capture

1. Sign in to the intended Myntra seller account and select a small representative set of
   orders. Include a multi-quantity order if the portal permits one and include every document
   the normal packing workflow prints.
2. Download the packed-orders CSV for the same orders so packet, release, order, tracking and
   seller-SKU identifiers can be compared without assuming they are equivalent.
3. Use the portal's normal Print action. In the browser or operating-system print dialog,
   select **Save to PDF** or **Print to File**, keep the original scale and orientation, disable
   browser-added headers and footers, and save the unmodified result.
4. If print-to-file is unavailable, use a controlled PDF virtual printer or CUPS capture queue.
   Record the browser, operating system, selected printer/driver and any scale or paper options.
   Do not photograph the page as a substitute for the captured payload.
5. Preserve the original filenames and files. Do not crop, combine, reorder, annotate or
   re-export them before analysis.

## Required evidence

- the original captured print file and its MIME type
- the matching packed-orders CSV
- which seller account produced it
- whether one Print action covered one order, a selected group or an entire batch
- whether the output contained labels, invoices or both
- any portal options that changed document content, order or paper size

Private analysis must record the SHA-256 checksum, page count, page boxes, text-extraction
availability and page order. It must then establish, from visible repeated identifiers, which
fields authoritatively associate label, invoice and CSV rows. Quantity must remain missing when
the capture does not prove it.

## Implementation gate

Only after this evidence is understood may a sanitized regression fixture be committed and
the isolated Myntra adapter gain PDF recognition, association or extraction. Print enrichment
also requires measured page geometry and a barcode-safe blank region. Capture, parsing,
generation, reprinting and physical printing remain Inventory-neutral.
