Implement CommerceOps Phase 8 — Returns and Cancellations.

Do not begin until Phase 7 is approved.

Use centralized Inventory from Phase 5.

Never maintain independent return-stock counters.

## Goal

Track ecommerce cancellations and returns and safely reconcile their inventory effects.

## Cancellation model

Support cancellation lifecycle with reference to the authoritative marketplace order.

Distinguish at minimum:

Cancellation before outbound:

* no stock deduction occurred
* therefore no restock transaction

Cancellation after outbound:

* inventory treatment must represent actual operational return/restock state
* do not blindly restock merely because cancellation status exists

Prevent duplicate cancellation processing.

## Return model

Track:

* marketplace
* marketplace order
* product
* expected quantity
* received quantity
* condition/disposition
* status
* timestamps
* actor
* notes/reason where applicable

Suggested statuses:

* expected
* received
* inspected
* restocked
* damaged/rejected
* closed

Use domain-appropriate final states based on approved workflow.

## Inventory integration

A return changes inventory only when the physical return has been accepted for restock.

Create:

`RETURN_RESTOCK`

transaction through Inventory.

Damaged/non-restockable returns must not increase available stock.

If damaged goods need separate treatment, use an explicit inventory transaction/disposition.

## Idempotency

The same return/cancellation event must never restock twice.

Use strong reference uniqueness/idempotency.

Retries must be safe.

## Reversal

Incorrect return decisions must be corrected using compensating inventory transactions rather than deleting history.

## Marketplace integration

Design returns around normalized marketplace orders so Flipkart and Amazon can share the domain.

Marketplace-specific status ingestion belongs in marketplace adapters.

Do not create independent return domains for each marketplace.

## Frontend

Provide:

Returns queue

* expected
* received
* needs inspection
* completed

Return detail

* source order
* products/quantities
* inventory impact
* status history

Cancellation queue/detail where required.

Provide clear actions such as:

* Mark Received
* Mark Restockable
* Restock
* Mark Damaged
* Close

Require confirmations for stock-changing actions.

## Reporting

Expose return/cancellation metrics to Phase 6 reporting.

Examples:

* returns received
* restocked quantity
* damaged quantity
* cancellations
* return rate

Do not build advanced profitability/accounting reports.

## Security

Use dedicated permissions such as:

* returns.view
* returns.manage
* returns.restock

Tenant isolation is mandatory.

## Tests

Cover:

* cancellation before outbound does not alter stock
* post-outbound workflow
* received return
* restock transaction
* damaged return does not increase stock
* duplicate restock prevented
* tenant isolation
* unauthorized restock
* partial quantity
* reversal/correction
* marketplace association
* concurrent actions
* dashboard reporting
* full inventory ledger reconciliation

Update docs and `CURRENT_STATE.md`.

STOP.

Do not begin Consignment/Phase 9.
