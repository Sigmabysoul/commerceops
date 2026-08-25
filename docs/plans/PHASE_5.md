COMMERCEOPS — PHASE 5: INVENTORY

ROLE

Implement the centralized CommerceOps Inventory domain.

This is a high-risk accounting-style business module.

Correctness, traceability and idempotency are more important than convenience or UI polish.

Phase 0–4 must already be approved.


==================================================
MANDATORY READING
==================================================

Read:

- AGENTS.md
- MASTER_SPEC.md
- ARCHITECTURE.md
- DOMAIN_RULES.md
- CURRENT_STATE.md
- Phase 5 documentation
- Product Master
- Flipkart order system
- Batch/printing implementation
- Audit system

Before coding, identify all existing paths that could affect stock.


==================================================
CORE INVENTORY RULE
==================================================

Never change inventory without an auditable inventory transaction.

Inventory must NOT behave as:

product.stock = product.stock - quantity

without recording the corresponding ledger transaction.


==================================================
CENTRALIZED INVENTORY
==================================================

There is one inventory domain shared by:

- ecommerce
- returns
- cancellations
- consignment
- manual stock-in
- corrections
- damages

Do not create separate independent stock counters for each module.


==================================================
LEDGER MODEL
==================================================

Create an immutable or strongly protected inventory transaction ledger.

Every transaction should conceptually include:

- id
- company
- product
- quantity delta
- transaction type
- reason
- source/reference type
- source/reference id
- actor/system
- timestamp
- metadata where justified


Example:

Product:
Averx Garbage Bag 3B

Delta:
-18

Type:
ECOMMERCE_OUT

Reference:
Flipkart Batch XYZ

Actor:
System

Timestamp:
...


==================================================
TRANSACTION TYPES
==================================================

Define a controlled set.

Initial conceptual types:

STOCK_IN
ECOMMERCE_OUT
RETURN_RESTOCK
CANCELLATION_RESTOCK
CONSIGNMENT_OUT
DAMAGED
MANUAL_ADJUSTMENT
CORRECTION

Only implement behavior currently supported.

It is okay to reserve types for future usage if clearly documented, but do not fake unsupported workflows.


==================================================
CURRENT STOCK
==================================================

Define clearly how current stock is obtained.

Potential architecture:

A. compute from ledger
or
B. maintain current balance plus immutable ledger transactionally

Choose based on current architecture/performance needs.

If using cached/current balances:

The balance update and ledger insertion MUST occur atomically in one database transaction.

Ledger remains the authoritative historical explanation.


==================================================
NO NEGATIVE STOCK
==================================================

Determine and document company policy.

Default safe behavior should prevent inventory from going below allowed values unless an authorized override exists.

If negative inventory is allowed in some situations, it must be explicit and auditable.

Do not silently allow accidental negative stock.


==================================================
STOCK IN
==================================================

Implement controlled stock-in.

User specifies:

- product
- quantity
- reason/source
- optional notes/reference

Create a positive inventory transaction.

Do not allow zero quantities.

Use clear validation for negative/positive semantics.


==================================================
MANUAL ADJUSTMENTS
==================================================

Manual adjustment is high-risk.

Require:

- permission
- reason
- auditable actor
- previous/resulting balance where useful

Do not implement manual editing of a raw stock number.

Adjust through a transaction.


==================================================
ECOMMERCE STOCK OUT
==================================================

Connect inventory to the existing batch/order workflow.

Important:

Do NOT deduct stock merely because:

- PDF uploaded
- label parsed
- label printed
- label reprinted

unless DOMAIN_RULES explicitly establish that printing is the business stock-out event.

Choose one explicit operational trigger.

Preferred design direction:

a clearly defined batch/order state representing stock physically leaving/being confirmed for outbound operation.

If current company workflow requires PRINTED as the trigger temporarily, make that an explicit documented rule and preserve the ability to change it later.

The trigger must be idempotent.


==================================================
IDEMPOTENCY
==================================================

This is mandatory.

For one business event:

Batch X → Ecommerce Out

running the operation twice must NOT create duplicate stock deductions.

Use strong database constraints/idempotency keys/reference uniqueness where appropriate.

Do not rely solely on:

"the button should only be clicked once."


==================================================
REPRINT SAFETY
==================================================

A reprint event must never affect stock.

Test this explicitly.

PrintJob and InventoryTransaction are independent concepts.


==================================================
PARTIAL/FAILED EVENTS
==================================================

Stock mutations must be transactional.

If an outbound operation contains multiple products and fails partway:

do not leave unexplained half-updated stock.

Use database transactions.

Define behavior for:

- partial batch
- failed deduction
- retry
- duplicate request


==================================================
STOCK RESERVATION FOUNDATION
==================================================

Phase 5 should establish reservation capability required by future Consignment.

Concepts:

Physical/on-hand
Reserved
Available

Available = On-hand - Reserved

Reservations must:

- belong to a company/product
- reference their source
- be traceable
- release correctly
- not become permanent orphan reservations

Do not implement the Consignment UI/workflow yet.

Create only a clean reusable reservation domain if Phase 5 scope/documentation approves it.


==================================================
INVENTORY BY COMPANY
==================================================

Stock is tenant-scoped.

One company's product/inventory must never be visible or mutable by another company.


==================================================
PRODUCT REFERENCES
==================================================

Inventory works only with canonical Product Master products.

Do not create stock by raw Flipkart SKU.

Flow:

Marketplace SKU
→ Product Master
→ Internal Product
→ Inventory


==================================================
AUDIT
==================================================

Inventory transactions already provide operational history, but administrative actions should also integrate with shared audit logging where appropriate.

Especially audit:

- manual adjustments
- corrections
- override allowing negative inventory
- reservation override
- administrative reversal


==================================================
REVERSALS / CORRECTIONS
==================================================

Do not delete historical inventory transactions merely because one was wrong.

Use compensating/reversal/correction transactions.

Example:

Incorrect:
-10

Correction:
+10

Then correct transaction:
-8

The history remains explainable.


==================================================
DELETE RULE
==================================================

Ordinary users must not be able to permanently delete inventory ledger history.

Do not expose generic DELETE inventory transaction APIs.


==================================================
CONCURRENCY
==================================================

Inventory must be safe when multiple operations happen simultaneously.

Example:

Current stock: 10

Two workers attempt outbound quantity 7 concurrently.

The application must not incorrectly allow both due to a race condition if negative stock is forbidden.

Use proper database transaction/locking/concurrency strategy.

Do not solve inventory correctness using only in-memory Go mutexes because CommerceOps may run multiple instances later.


==================================================
API
==================================================

Implement inventory APIs needed now.

Examples conceptually:

GET /inventory
GET /inventory/products/:id
GET /inventory/transactions
POST /inventory/stock-in
POST /inventory/adjustments
POST /inventory/outbound/... if appropriate
GET /inventory/reservations
POST/DELETE reservation operations through domain-specific commands

Use command-oriented endpoints where they better express business actions.

Avoid exposing a generic "set stock = X" endpoint.


==================================================
FRONTEND
==================================================

Build functional inventory screens.

Inventory Dashboard:
- product
- on-hand
- reserved
- available

Product inventory detail:
- current balance
- transaction history

Stock In:
- product
- quantity
- reason

Adjustment:
- product
- quantity/delta
- required reason
- confirmation

Filters:
- date
- transaction type
- product

Do not build advanced analytics yet.


==================================================
PERMISSIONS
==================================================

Use granular permissions such as:

inventory.view
inventory.stock_in
inventory.adjust

If ecommerce completion automatically creates stock movement, that system operation must still be authorized by valid workflow/state, not by a user's ability to forge API data.


==================================================
REPORTING PREPARATION
==================================================

Expose/structure inventory data so Phase 6 reporting can calculate:

- stock in today
- stock out today
- net movement
- product movement
- current stock
- shortages

Do not implement full reporting dashboard yet.


==================================================
TESTING — CRITICAL
==================================================

Inventory tests are mandatory.

At minimum test:

1. stock-in creates correct positive transaction
2. outbound creates correct negative transaction
3. current balance is correct
4. duplicate outbound event does not deduct twice
5. reprint does not affect inventory
6. manual adjustment requires reason
7. unauthorized adjustment blocked
8. tenant isolation
9. concurrent outbound cannot corrupt balance
10. failed transaction rolls back completely
11. reversal/correction preserves original history
12. deletion of ledger history unavailable
13. negative stock policy enforced
14. reservation reduces available but not on-hand
15. reservation release restores available
16. consuming reservation behaves correctly if implemented
17. product must be canonical Product Master ID
18. zero/invalid quantity rejected
19. audit generated for manual correction where applicable
20. CI/integration tests use real PostgreSQL where correctness requires DB transaction behavior


==================================================
LOAD / PERFORMANCE CHECK
==================================================

Perform a reasonable test using many inventory transactions.

Do not optimize prematurely.

Check for obvious problems such as:

- N+1 queries
- full-table scans
- missing indexes
- loading complete ledger into memory for simple totals

Report observations.


==================================================
DO NOT IMPLEMENT
==================================================

Do not implement:

- Return workflow UI
- cancellation workflow
- full consignment system
- Amazon
- advanced reports
- automatic purchasing
- forecasting
- accounting/financial ledger
- supplier management


==================================================
DEFINITION OF DONE
==================================================

Phase 5 is implementation-complete when:

- centralized inventory exists
- transaction ledger exists
- stock-in works
- manual adjustment works safely
- ecommerce outbound integration works according to documented trigger
- duplicate deductions are impossible under tested conditions
- reprinting does not change inventory
- current/on-hand balance is accurate
- reservation foundation works if included
- concurrency is handled at database level
- correction/reversal approach exists
- tenant isolation works
- permissions work
- tests are comprehensive
- docs updated


==================================================
COMPLETION REPORT
==================================================

Report in detail:

1. inventory schema
2. balance calculation strategy
3. transaction types
4. exact ecommerce stock-out trigger
5. idempotency implementation
6. concurrency strategy
7. reservation model
8. reversal/correction design
9. indexes/performance decisions
10. APIs
11. frontend
12. tests
13. load/performance observations
14. dependencies
15. known risks
16. documentation changes

Finish with:

PHASE 5 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 5 NOT COMPLETE

STOP.
Do not begin Phase 6.