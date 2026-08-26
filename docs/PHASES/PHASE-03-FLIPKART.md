COMMERCEOPS — PHASE 3: FLIPKART PROCESSING

ROLE

Implement the first marketplace adapter: Flipkart.

This is the first real ecommerce processing module.

Phase 0, 1 and 2 must already be approved.


==================================================
MANDATORY READING
==================================================

Read all required project documentation plus:

- marketplace architecture documentation
- docs/marketplaces/flipkart.md
- Product Master documentation
- current Phase 2 implementation

Inspect existing known Flipkart processing logic if legacy scripts are available, but do not blindly copy their architecture.


==================================================
PHASE GOAL
==================================================

CommerceOps should accept Flipkart label/document files and convert them into normalized, traceable internal order/label records.

This Phase focuses on:

- file upload
- PDF inspection
- label/page extraction
- Flipkart identification
- AWB/order identifier extraction
- marketplace SKU extraction
- quantity extraction where reliably available
- Product Master resolution
- duplicate detection
- processing status
- errors/manual review

Phase 3 does NOT perform final batch printing or stock deduction.


==================================================
MARKETPLACE ADAPTER PRINCIPLE
==================================================

All Flipkart-specific knowledge must stay inside the Flipkart marketplace module.

The rest of CommerceOps should consume normalized internal structures.

Do not scatter strings like:

"Flipkart"
"Consignment ID"
specific regex patterns
specific PDF coordinates

through unrelated modules.


==================================================
NORMALIZED OUTPUT
==================================================

A successful Flipkart processor should produce normalized information conceptually including:

- company
- marketplace = flipkart
- marketplace order ID where available
- AWB/tracking ID
- marketplace SKU/raw identifier
- resolved internal product if known
- quantity
- source file/page
- raw extraction metadata if required
- processing status
- warnings/errors


==================================================
FILE UPLOAD
==================================================

Implement secure upload.

Requirements:

- validate expected file type
- enforce reasonable size limits
- do not trust filenames
- generate safe server-side identifiers
- protect tenant ownership
- calculate file hash if useful for duplicate source-file detection

Do not permanently store large PDFs inside PostgreSQL.

Use the approved file storage abstraction.

Local development may use an appropriate local-compatible implementation if documented.


==================================================
PDF PROCESSING
==================================================

Use Go where practical.

If the existing PDF workload genuinely requires Python/PyMuPDF due to library capability, do NOT silently introduce Python.

First document the limitation.

Follow the approved specialized-worker architecture.

Python may handle document processing but must not own:

- orders
- authorization
- inventory
- company logic
- business persistence rules

The Go backend remains the orchestration/business system.


==================================================
BACKGROUND JOBS
==================================================

Large Flipkart processing must not occur inside a long HTTP request.

Workflow:

Upload file
→ create processing job
→ return quickly
→ background processing
→ progress/state persisted
→ frontend checks progress
→ complete/error/manual review

Use a bounded worker model.

Do not introduce Redis/Kafka solely for this.

A PostgreSQL-backed job approach or controlled in-process worker may be used according to project architecture.


==================================================
PROCESSING STATES
==================================================

Use explicit states.

Conceptual examples:

UPLOADED
QUEUED
PROCESSING
NEEDS_REVIEW
PROCESSED
FAILED

Do not silently skip pages/labels.

If a page cannot be processed, retain an error/warning tied to it.


==================================================
DUPLICATE DETECTION
==================================================

Use Flipkart identifiers such as AWB/order ID where reliable.

Duplicate protection is critical.

Uploading the same order twice should be detected.

Do not yet create stock deductions.

Duplicates should be visible to the user.

Distinguish:

- same file uploaded again
- different file containing same AWB/order


==================================================
PRODUCT RESOLUTION
==================================================

Extract raw Flipkart SKU/product identifier.

Pass it through Product Master.

Results:

RESOLVED
UNRESOLVED
AMBIGUOUS if architecture permits

Do not silently invent products.

Unknown SKU should be available for Product Training.

Once trained, reprocessing/resolution should be possible.


==================================================
QUANTITY
==================================================

Extract quantity only when the source provides a reliable basis.

Do not default silently to quantity = 1 if extraction failed.

If a business rule requires a fallback, represent:

- extracted quantity
- fallback/default status
- warning

so users can distinguish known values from assumptions.


==================================================
KNOWN FLIPKART DOCUMENT NEEDS
==================================================

The project's Flipkart workflow has previously dealt with concepts such as:

- AWB
- order ID
- consignment ID
- Box ID
- marketplace SKU
- quantity
- shipping label
- invoice portions
- multi-label pages

Implement based on actual supplied/test fixture formats.

Do not over-generalize from one PDF without tests.


==================================================
TEST FIXTURES
==================================================

Create sanitized/non-sensitive fixture documents if practical.

Tests should cover different actual Flipkart document shapes.

Do not embed private customer information into the public repository.


==================================================
ORDER/LABEL DATA MODEL
==================================================

Introduce only the normalized marketplace/order/label schema needed now.

Likely concepts:

marketplace_accounts if justified
orders
order_items
source_files
labels
processing_jobs
processing_errors

Avoid designing all future marketplace complexity in one phase.


==================================================
MANUAL REVIEW
==================================================

Uncertain processing should be recoverable.

Provide a manual-review state for:

- unknown SKU
- missing AWB
- missing quantity
- malformed document
- duplicate
- unsupported label format

A user should be able to see WHY an item requires review.


==================================================
FRONTEND
==================================================

Implement a practical Flipkart processing UI.

Suggested flow:

Flipkart
→ Upload
→ Processing job
→ Progress
→ Results

Results should show:

- successful
- unresolved
- duplicate
- failed
- product quantities if available
- raw SKU → internal Product
- warnings

Provide links/actions to Product Training for unknown SKUs where appropriate.


==================================================
AUTHORIZATION
==================================================

Use module entitlements.

Company must have Flipkart module enabled.

Use permissions such as:

labels.upload
labels.process

Do not rely on UI visibility for authorization.


==================================================
AUDIT
==================================================

Audit important actions:

- file uploaded
- processing started/completed
- manual correction
- duplicate override if one is ever allowed

Automated detailed extraction logs need not all become permanent audit events.


==================================================
TESTING
==================================================

Test at minimum:

1. company without Flipkart entitlement cannot process
2. user without upload permission is blocked
3. valid Flipkart fixture processes
4. AWB/order extraction
5. SKU extraction
6. known SKU resolves
7. unknown SKU needs review
8. duplicate AWB detection
9. duplicate source file detection where implemented
10. quantity failure does not silently become trusted data
11. malformed PDF fails safely
12. tenant isolation
13. background job state transitions
14. page/label errors are visible
15. no stock transaction is created


==================================================
NON-GOALS
==================================================

Do NOT implement:

- final batch orchestration
- final print queue
- stock deduction
- returns
- Amazon
- Meesho
- Myntra
- Snapdeal
- automatic Flipkart portal login/download
- OCR/AI unless absolutely required and approved


==================================================
DEFINITION OF DONE
==================================================

Phase 3 is implementation-complete if:

- Flipkart PDFs can be uploaded
- processing happens safely
- normalized orders/items are produced
- AWB/identifiers extracted reliably for supported fixtures
- SKUs resolve through Product Master
- unknown SKUs are visible
- duplicate protection works
- processing errors are visible
- tenant/permission/module checks work
- tests cover actual fixture behavior
- no stock movement occurs
- docs are updated


==================================================
COMPLETION REPORT
==================================================

Report:

1. Flipkart formats supported
2. extraction approach
3. PDF libraries/dependencies
4. schema changes
5. processing state machine
6. duplicate strategy
7. background-job implementation
8. Product Master interaction
9. known unsupported formats
10. tests/fixtures
11. performance observations
12. documentation changes

Finish with:

PHASE 3 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 3 NOT COMPLETE

STOP.
Do not begin Phase 4.