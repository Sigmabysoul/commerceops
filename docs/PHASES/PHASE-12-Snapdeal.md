Implement CommerceOps Phase 12 — Snapdeal Marketplace Processing.

DO NOT begin until the currently active prior phase is formally approved.

This Phase 12 prompt is based on a real representative Snapdeal document sample.
Do not replace the observed structure with assumptions from Flipkart/Amazon/Meesho.

Read first:
- AGENTS.md
- docs/AI_WORKFLOW.md
- docs/CURRENT_STATE.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/MODULES.md
- Product Master implementation
- shared Marketplace infrastructure
- Batch + Printing implementation
- Inventory
- Reporting
- Returns
- Amazon enrichment implementation
- Meesho adapter
- the provided private Snapdeal PDF sample

GOAL

Add Snapdeal as an isolated marketplace adapter that reuses the existing
CommerceOps marketplace pipeline.

Snapdeal should NOT be treated like Flipkart cropping.

The observed workflow is closer to Amazon-style document association and
shipping-label enrichment.

--------------------------------------------------
OBSERVED REAL SNAPDEAL DOCUMENT STRUCTURE
--------------------------------------------------

Representative sample contains 2 pages.

PAGE 1 — SHIPPING / PACKSLIP STYLE PAGE

Observed fields include:
- Snapdeal branding
- courier name
- courier barcode
- delivery address
- SUBORDER CODE
- seller
- GSTIN
- QUANTITY
- shipped-from address
- Snapdeal Reference No barcode
- a compact product/SKU-like code below the suborder

Observed example:
SUBORDER CODE:
74088053700

Compact code:
7_SESTNAFR1B1

Quantity:
1

PAGE 2 — TAX INVOICE

Observed fields include:
- invoice number
- invoice date
- order date
- seller
- buyer
- item description
- explicit quantity
- SKU CODE
- SUBORDER
- HSN
- tax values

Observed example:
SKU CODE: 7_SEST-NAF-R1-B-1
SUBORDER : 74088053700
QTY: 1

--------------------------------------------------
PRIMARY ASSOCIATION RULE
--------------------------------------------------

Use SUBORDER / SUBORDER CODE as the primary document-association key when
associating invoice information to the shipping/packslip page.

Do NOT use page position as the authoritative association rule when a stable
suborder identifier is available.

If multiple pages contain the same suborder, association must be deterministic
and ambiguity must enter review rather than being guessed.

--------------------------------------------------
SKU EXTRACTION
--------------------------------------------------

Preferred authoritative raw seller SKU source:

1. Explicit invoice field:
   SKU CODE: <value>

Example:
7_SEST-NAF-R1-B-1

2. The compact code visible on the shipping/packslip page may be retained as
   supplemental evidence if useful, but do not assume it is always exactly the
   same normalized string as the invoice SKU.

Observed example difference:
Shipping page compact form:
7_SESTNAFR1B1

Invoice:
7_SEST-NAF-R1-B-1

Do NOT normalize by inventing punctuation unless a deterministic, tested rule is
proven from real samples.

The Product Master remains authoritative.

Flow:
raw Snapdeal SKU
→ marketplace SKU mapping
→ canonical Product Master product

Unknown SKU:
→ unresolved / review

Never auto-create canonical products.

--------------------------------------------------
QUANTITY
--------------------------------------------------

Use explicit positive quantity only.

Observed quantity appears both on:
- page 1 QUANTITY
- page 2 invoice QTY

Do not silently default quantity to 1.

If quantity is missing:
→ unresolved/review

If shipping and invoice quantities conflict:
→ review

If quantity is malformed or zero/non-positive:
→ review

--------------------------------------------------
DOCUMENT CLASSIFICATION
--------------------------------------------------

Create isolated Snapdeal classification logic.

Conceptual roles:
- shipping_label
- invoice

Do not hardcode a generic even-page/odd-page rule.

Classification must use observed textual/document evidence.

Potential shipping evidence:
- DELIVERY ADDRESS
- SUBORDER CODE
- SHIPPED FROM
- courier/tracking barcode area
- Snapdeal Reference No

Potential invoice evidence:
- TAX INVOICE
- INVOICE NUMBER
- SKU CODE
- SUBORDER
- ITEM DESCRIPTION
- HSN

If classification is ambiguous:
→ review

--------------------------------------------------
PARSER TRACEABILITY
--------------------------------------------------

Every extracted record must preserve:
- source file ID
- page number
- parser version
- extraction method
- document role
- normalized marketplace order
- raw values
- Product Master resolution result
- review/failure reason

Create a Snapdeal parser version such as:
snapdeal-packslip-v1

Only use a different version if repository naming conventions require it.

--------------------------------------------------
PRINTING BEHAVIOR
--------------------------------------------------

Snapdeal does NOT require Flipkart-style cropping based on the supplied sample.

The expected print behavior is closer to Amazon shipping-label enrichment.

Desired output:
take the shipping/packslip page and visibly add:

SKU: <canonical or approved display SKU> | QTY: <explicit quantity>

Rules:
- do not obscure courier barcode
- do not obscure Snapdeal Reference No barcode
- do not obscure delivery address
- do not obscure suborder information
- preserve machine-readable barcode quality
- preserve the original shipping page content
- no inventory movement

Because the representative shipping page already contains meaningful whitespace
outside the main label area, first inspect the real page geometry and choose a
safe deterministic placement.

Do NOT invent overlay coordinates before measuring the actual sample.

Use the existing PDF-generation abstractions.

If a specialized Snapdeal print adapter is required, place it in the isolated
Snapdeal marketplace adapter rather than generic infrastructure.

--------------------------------------------------
BATCH + PRINTING
--------------------------------------------------

Snapdeal must participate in the existing batch system.

Reuse:
- normalized marketplace orders
- Product Master totals
- sorting
- assignments
- ready state
- print artifacts
- print/reprint history
- downloads

Do NOT build a Snapdeal-specific batch system.

Reprinting must remain inventory-neutral.

If invoice export is supported, keep invoice output separate from shipping-label
output using the same normalized association.

Do not guess invoice association when evidence is incomplete.

--------------------------------------------------
INVENTORY
--------------------------------------------------

Absolutely no stock movement during:

upload
parse
classification
association
Product Master resolution
batch creation
PDF generation
printing
reprinting

Only existing explicit ecommerce outbound confirmation may create:

ECOMMERCE_OUT

Use the existing central Inventory domain.

Do not add Snapdeal-specific stock counters or stock tables.

--------------------------------------------------
RETURNS
--------------------------------------------------

Resolved Snapdeal marketplace orders must work with the existing normalized
Returns/Cancellations domain.

Do not create a Snapdeal-specific return engine.

Cancellation/return behavior should be inherited through normalized orders.

--------------------------------------------------
REPORTING
--------------------------------------------------

Expose Snapdeal naturally through existing marketplace-aware reporting.

Snapdeal filtering should correctly isolate:
- processed orders
- batches
- print runs
- outbound-confirmed orders
- unresolved/review
- returns/cancellations tied to Snapdeal

Company-wide inventory balances, stock-in, generic adjustments, corrections and
consignment semantics should remain company-wide where existing reporting rules
define them that way.

Do not create independent reporting counters.

--------------------------------------------------
SECURITY
--------------------------------------------------

Use:
- authenticated company context
- Snapdeal module entitlement
- existing marketplace upload/process permissions
- batch/printing permissions
- existing Returns permissions

Never trust company_id from the client.

Tenant isolation mandatory.

--------------------------------------------------
PRODUCTION DATA / FIXTURES
--------------------------------------------------

The provided Snapdeal PDF is private production material.

Do NOT commit:
- customer names
- addresses
- phone numbers
- actual business references
- GST details
- live barcodes

Create sanitized regression fixtures preserving the structural layout and field
patterns.

Keep a private-sample regression hook similar to the Meesho/Amazon pattern.

--------------------------------------------------
TESTS
--------------------------------------------------

Cover at minimum:

PARSER
- shipping-page classification
- invoice-page classification
- explicit SUBORDER extraction
- explicit SKU CODE extraction
- explicit quantity extraction
- no quantity default
- malformed quantity
- conflicting quantity
- ambiguous SKU
- missing suborder
- unsupported document

ASSOCIATION
- shipping/invoice same SUBORDER
- mismatched suborder
- duplicate suborder
- ambiguous association
- no position-based guessing when identifier exists

PRODUCT MASTER
- mapped SKU
- unknown SKU
- inactive mapping if relevant
- tenant isolation

BATCH / PRINTING
- Snapdeal eligible-order listing
- batch creation
- deterministic sorting
- generated shipping-label artifact
- visible SKU/QTY enrichment
- barcode preservation validation where feasible
- invoice export if implemented
- reprint idempotency
- reprint inventory neutrality

INVENTORY
- upload neutral
- parse neutral
- batch neutral
- print neutral
- reprint neutral
- explicit outbound creates ecommerce_out once
- duplicate outbound prevented

RETURNS
- Snapdeal order accepted by shared cancellation/return workflow
- no Snapdeal-specific inventory logic

REPORTING
- Snapdeal marketplace filter isolation
- outbound metrics
- return/cancellation metrics
- no cross-market leakage

SECURITY
- tenant isolation
- entitlement denial
- permission denial

DATABASE
- migration up/down if schema changes
- PostgreSQL concurrency/idempotency where required

FULL VERIFICATION
- Go tests
- go vet
- Go build
- frontend typecheck
- frontend lint
- frontend production build
- OpenAPI parsing
- git diff --check
- make verify-full with TEST_DATABASE_URL

--------------------------------------------------
IMPLEMENTATION WORKFLOW
--------------------------------------------------

Before modifying code:

1. Read authoritative docs.
2. Inspect existing marketplace infrastructure.
3. Inspect Amazon enrichment logic for reusable concepts.
4. Inspect the provided private Snapdeal sample.
5. Produce a PLAN containing:
   - exact files expected to change
   - schema/API changes
   - generic reuse vs Snapdeal-specific logic
   - parser/classification design
   - association design
   - printing/enrichment design
   - security risks
   - regression strategy
6. Confirm Phase 12 scope only.

Then implement in medium cohesive batches.

Recommended:

Batch A
- Snapdeal adapter
- upload/process integration
- classification
- extraction
- Product Master resolution
- traceability
- sanitized/private parser tests

Batch B
- deterministic invoice/shipping association
- batch integration
- Snapdeal print enrichment
- optional invoice artifact
- print/reprint regression coverage

Batch C
- Inventory outbound integration
- Returns compatibility
- reporting/dashboard
- frontend marketplace integration
- comprehensive PostgreSQL/CI verification
- docs/CURRENT_STATE

At the end report:

Changed
Behavior
Database / Migration Impact
Tests
Not Tested
Risks / Limitations
Scope Confirmation
Branch / Commit / Working Tree

IMPORTANT LIMITATION

One real Snapdeal sample is enough to establish the observed baseline, but do
not claim universal support for every Snapdeal document layout.

Document any untested layout variants explicitly.

STOP after Phase 12.

Do not begin Phase 13 Printing Platform automatically.
