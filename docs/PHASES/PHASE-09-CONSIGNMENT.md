Implement CommerceOps Phase 9 — Consignment Management.

Do not begin until Phase 8 is approved.

Use Product Master, Inventory, employee/role permissions, audit logging and reservation infrastructure already present.

## Goal

Digitize the company's consignment workflow from sales/order requirement through departmental preparation, packing and outbound completion.

## Core concepts

Model consignment records containing:

* consignment/order reference
* dealer/customer reference where appropriate
* required products
* quantities
* department/team responsibility
* pouch/file number where applicable
* workflow status
* timestamps
* actors
* notes
* inventory reservations

Do not represent the workflow only as freeform spreadsheet cells.

## SO

Support the existing operational concept of an SO:

A source/order document describing:

* order/reference ID
* products required
* required quantities

Preserve traceability to imported/manual source data.

## Pouch / file number

Support the operational POUCH/file number as a first-class configurable reference.

Do not assume it is globally unique unless the real workflow confirms that.

Enforce uniqueness in the appropriate company/time/domain scope.

## Department views

Support department-scoped working views.

Initial operational examples may include:

* GB
* AK
* Mixed

Do NOT encode those values as architectural constants.

Represent departments/teams as configurable data.

A department worker should be able to focus on products/actions relevant to that department.

## Roles

Integrate with existing role/permission system.

Conceptual permissions:

Developer/Admin/HR/Consignment Head:

* broad visibility and management

Team Leader:

* operational visibility across authorized departments
* status updates

Workers:

* scoped views/actions according to assigned department/role

Do not hardcode specific employee names.

## Workflow

Define explicit states, for example:

* created
* allocated
* picking
* ready
* packing
* packed
* outbound
* completed
* cancelled

Finalize exact state machine from real workflow.

Prevent invalid transitions.

Record actors/timestamps.

## Product progress

Each consignment product line should support operational progress.

Workers should be able to mark items such as:

* pending
* ready
* packed

Department views should clearly show outstanding work.

## Inventory reservation

When a consignment becomes operationally confirmed, reserve required canonical Product Master quantities using Phase 5 reservation infrastructure.

Reservation:

* reduces Available
* does not reduce On-hand

When consignment is confirmed outbound:

consume/release reservation according to Inventory domain and create:

`CONSIGNMENT_OUT`

Do not deduct raw stock independently.

Cancellation releases reservations safely.

## Partial fulfillment

Support real partial cases deliberately.

Do not mark entire consignment completed if required lines remain incomplete unless an authorized exception exists.

## Concurrency

Multiple workers may update different lines simultaneously.

Use database-safe state changes and optimistic/transactional conflict handling.

## UI

Prioritize readability and simplicity.

Users may include staff who prefer large, clear controls over dense interfaces.

Provide:

Consignment board

* reference
* pouch/file
* progress
* department
* status

Consignment detail

* product lines
* quantity
* department
* ready/packed status
* progress summary

Department views

* only relevant work by default
* authorized users can switch to all/mixed views

Avoid building a spreadsheet clone unless a table is clearly the best interaction for a specific screen.

## Audit

Audit:

* creation
* assignment
* status transition
* quantity changes
* reservation
* cancellation
* outbound confirmation
* administrative overrides

## Reporting

Expose:

* pending consignments
* completed today
* product quantities
* department workload
* average completion timing where reliable
* consignment inventory movement

to the existing reporting domain.

## Tests

Cover:

* tenant isolation
* role/permission differences
* department scoping
* state machine
* product progress
* reservation creation
* reservation release
* outbound inventory deduction
* duplicate outbound protection
* cancellation
* partial fulfillment
* concurrent updates
* canonical Product Master usage
* audit records
* reporting integration

Update docs and `CURRENT_STATE.md`.

STOP.

Do not begin Meesho/Phase 10.
