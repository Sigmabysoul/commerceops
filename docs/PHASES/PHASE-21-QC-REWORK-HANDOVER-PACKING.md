# Phase 21 — QC, rework, handover and packing

Status: Implementation active — owner authorized 2026-09-11; completion requires fresh verification.
Revision: 2026-09-11.

## Goal and work

Define explicit worker workflows, quantities, custody transfers, sticker/rework steps and packing/final checks over Traceability.

## Boundaries

Server-authoritative transitions; bounded quantities and auditable actions. QC PASS never automatically restocks.

## Acceptance and verification

Invalid transitions, concurrent updates, idempotent mobile retries, custody and Inventory neutrality.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.

## Implemented contract pending completion gate

- QC is one immutable snapshot covering every current Product quantity in the box.
- Rejected quantity records a controlled reason and creates one typed work requirement. Work
  completion does not pass QC; a fresh full passing check is required before packing.
- Handover send and receive are separate events. A box remains in transit and blocks other
  mutations until the target employee or a member of the target department receives it.
- Packing, final check and shipment readiness are separate ordered events. A failed final check
  blocks readiness and requires packing/final check to be recorded again.
- All commands use company scope, entitlement, permission, row locking and idempotency keys.
  They append audit/history records and never mutate Inventory.
- Returns and Consignment associations, pouch/file evidence and mixed-department progress remain
  Phase 22 or later work.
