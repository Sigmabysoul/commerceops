# Phase 22 — Consignment traceability integration

Status: Implementation active — owner authorized 2026-09-11; completion requires fresh verification.
Revision: 2026-09-11.

## Goal and work

Connect Traceability to Consignment lines, department progress, packing and pouch/file evidence, including mixed-department work.

## Boundaries

Preserve Product truth, department snapshots and Inventory reservation/outbound ownership. Pouch/file references are not globally unique by assumption.

## Acceptance and verification

Mixed consignments, progress snapshots, packing completeness, reservation/outbound replay and company isolation.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.

## Implemented contract pending completion gate

- Consignments explicitly opt into Traceability enforcement; existing records retain Phase 9 behavior.
- Immutable signed allocations link a shipment-ready Trace Box quantity to a canonical Consignment
  line. Box and Consignment locks prevent double allocation across concurrent requests.
- Ready/packed progress and ready, packed and outbound gates revalidate current box eligibility and
  complete line coverage. Inventory reservation and outbound remain unchanged.
- Department progress derives from line department snapshots, including mixed-department work.
- Pouch and file reference evidence is append-only, company-scoped and deliberately non-unique.
