COMMERCEOPS — PHASE 4: BATCH AND PRINTING

ROLE

Implement CommerceOps batch organization and printing workflow.

Phase 3 Flipkart processing must already be approved.

Do not implement inventory deductions yet.


==================================================
MANDATORY READING
==================================================

Read:

- AGENTS.md
- MASTER_SPEC
- ARCHITECTURE
- DOMAIN_RULES
- CURRENT_STATE
- Phase 4 documentation
- Flipkart processing implementation
- Product Master implementation
- employee/permission implementation


==================================================
PHASE GOAL
==================================================

Take normalized processed ecommerce labels/orders and organize them into operational batches that can:

- group labels/orders
- calculate product totals
- assign work
- sort labels
- create print-ready output
- track printing
- track reprinting
- expose processing/printing progress

Inventory must NOT yet be deducted.


==================================================
BATCH DOMAIN
==================================================

Create an explicit Batch entity/domain.

A batch should represent an operational group of orders/labels.

Conceptual fields:

- id
- company
- marketplace
- created_at
- status
- source processing job(s)
- order/label count
- created_by
- completion information

Do not store all calculated summaries as unrelated manual counters unless required for performance.

Authoritative values should come from batch/order relationships.


==================================================
BATCH STATES
==================================================

Define an explicit state machine.

Example concept:

DRAFT
READY
PRINT_QUEUED
PRINTING
PRINTED
COMPLETED
CANCELLED
FAILED

Use only states that have clear meaning.

Document allowed transitions.

Do not allow random status changes.


==================================================
BATCH MEMBERS
==================================================

Orders/labels associated with a batch must be traceable.

Prevent accidental duplicate inclusion unless explicitly allowed by domain rules.

A label/order should not silently be printed through multiple operational batches in a way that creates confusion.


==================================================
PRODUCT TOTAL CALCULATION
==================================================

For each batch calculate:

- internal product
- total quantity
- number of order lines
- unresolved quantities if any

Product totals must use normalized Product Master IDs, not raw marketplace text.

Examples:

Averx Garbage Bag 3B → 31
Averx Garbage Bag 2B → 22
Butter Paper → 12


==================================================
WORKER ASSIGNMENT
==================================================

Implement generic assignment rules if not already available.

Initial company configuration:

Kartik currently handles:

Garbage Bags:
- 2 Bag
- 3 Bag
- brands Averx, Star, Plain

Garbage Rolls:
- 17x19
- 19x21

Butter Paper

Aluminium Containers:
- 25 pc
- 50 pc
- sometimes 100 pc

Default unassigned work is currently handled by Sohel.

CRITICAL:

These assignments must exist as database/configuration rules.

Do not write code such as:

if product == garbage_bag:
    employee = kartik


==================================================
ASSIGNMENT RULE DESIGN
==================================================

Rules should support future growth.

Conceptual rule criteria may include:

- specific product
- product group/category if available
- marketplace
- default fallback
- priority

Keep the first version understandable.

Do not build a complicated generic rules engine unless necessary.


==================================================
SORTING
==================================================

Allow batches/print jobs to sort labels into useful sequences.

Potential sorting dimensions:

- assigned worker
- product
- SKU
- marketplace
- configured sort key

The sorting system should be deterministic.

Do not hardcode one company's product ordering in general application logic.

Store configurable sort preference/order where justified.


==================================================
PRINT-READY DOCUMENT
==================================================

Generate print-ready label output based on processed labels.

The output may:

- crop unnecessary invoice portions where approved
- arrange labels appropriately
- include approved label modifications
- preserve important marketplace identifiers/barcodes
- maintain readable print quality

Do not silently alter legally/operationally important marketplace information.


==================================================
FLIPKART LABEL MODIFICATIONS
==================================================

Existing company needs may include:

- label cropping
- Box ID readability
- larger/regenerated barcode
- consignment ID placement
- preserving shipping details
- avoiding invoice/unused portions

Implement only modifications that are documented and validated against sample labels.

Use proper PDF coordinates derived from supported formats.

Do not assume all Flipkart documents have identical geometry.


==================================================
PRINT JOB DOMAIN
==================================================

Printing must be tracked separately from batch status.

Introduce PrintJob or equivalent.

A print job should conceptually record:

- company
- batch
- requested output
- requested_by
- printer target if applicable later
- status
- created time
- completed time
- failure information


==================================================
PRINT STATES
==================================================

Conceptually:

QUEUED
READY
PRINTING
PRINTED
FAILED
CANCELLED

If actual browser printing prevents knowing physical completion, distinguish system-side generation/readiness from confirmed physical printing.

Do not claim physical print success unless it can actually be known.


==================================================
REPRINTING
==================================================

Reprinting is a first-class action.

Record:

- which label/batch
- who requested it
- when
- reason if appropriate
- source print job

CRITICAL:

Reprinting must NEVER create a second inventory deduction later.

Design print identity separately from stock movement identity.


==================================================
DESKTOP AGENT
==================================================

Do NOT build the advanced printer agent in Phase 4.

For now:

- generate downloadable/printable output
- use browser/native print flow as needed
- model PrintJob in a way a desktop agent can consume later

Do not add Tauri/Rust unless explicitly approved.


==================================================
FRONTEND
==================================================

Build practical batch/printing screens.

Examples:

Batch List

Batch Detail:
- marketplace
- order count
- label count
- product totals
- worker totals
- unresolved issues
- status

Print:
- generate
- preview/download
- mark/track appropriate state
- reprint

Worker views:
- assigned product totals


==================================================
ERRORS
==================================================

Do not allow batch finalization when critical unresolved items remain unless an explicit authorized override exists.

Examples:

- unknown product
- invalid quantity
- missing required label
- processing failure

Show clear blocking reasons.


==================================================
AUTHORIZATION
==================================================

Use:

labels.process
labels.print
labels.reprint

and other existing permissions.

Module entitlement still applies.


==================================================
AUDIT
==================================================

Audit:

- batch created
- batch cancelled
- worker assignment override
- print requested
- reprint
- important manual corrections


==================================================
TESTING
==================================================

Test at minimum:

1. batch product totals
2. batch cannot include unauthorized tenant data
3. duplicate inclusion protection
4. worker assignment rule resolution
5. fallback assignment
6. assignments are configuration, not hardcoded
7. deterministic sorting
8. print-ready output generated for supported fixture
9. reprint creates print history
10. reprint does not create stock movement
11. permission enforcement
12. state transition validation
13. unresolved critical item blocks completion where required
14. audit events for reprint/overrides
15. large batch processing remains bounded


==================================================
NON-GOALS
==================================================

Do not implement:

- inventory deduction
- returns
- consignment
- Amazon
- automatic printer agent
- direct printer control
- billing


==================================================
DEFINITION OF DONE
==================================================

Phase 4 is complete when:

- batches work
- product totals are accurate
- worker assignment is configurable
- sorting works
- print-ready PDFs work for supported Flipkart formats
- print jobs are tracked
- reprints are tracked
- state machine is enforced
- unresolved issues are handled safely
- no inventory is deducted
- tests pass
- docs updated


==================================================
COMPLETION REPORT
==================================================

Report:

1. batch model
2. state machine
3. worker assignment design
4. sorting behavior
5. PDF/printing implementation
6. print tracking
7. reprint guarantees
8. schema changes
9. tests
10. supported label formats
11. known limitations
12. documentation updates

Finish:

PHASE 4 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 4 NOT COMPLETE

STOP.
Do not begin Phase 5.