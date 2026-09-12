# Superseded planning reference

Preserved from the pre-consolidation working tree on 2026-09-07. Phase numbers and authorization statements below are superseded by `../../ROADMAP.md`. This file does not authorize implementation.

# Phase 17 — Return Rework & Box Traceability

**Status:** PLANNED / NOT IMPLEMENTED

## Goal

Track returned/reworked physical goods across QC, department handover,
rework/sticker/preparation, packing, final verification and shipment readiness.

## Trace Box

Each large physical box receives a random opaque QR/token.

QR contains only the opaque identifier.
All operational truth remains server-side.

Scan still requires authenticated/authorized access.

## Box contents

Use canonical Product Master IDs and explicit quantities.

Support eventually:
- add/remove
- quantity correction with reason
- split
- merge
- transfer

All changes append history.

## QC

Record:
- employee
- checked quantity
- passed
- rejected
- rejection reasons
- timestamps
- notes

Possible reasons:
- damaged
- wrong sticker
- dirty
- missing component
- packaging damaged
- wrong product
- manufacturing defect
- other

No quantity default.

## Handover

Separate:
- HANDOVER_SENT
- HANDOVER_RECEIVED

Until receiver confirms, box can be IN_TRANSIT.

## Work requirements

Examples:
- sticker replacement
- cleaning
- repacking
- component check
- final check

Required work blocks later workflow gates.

## Packing / final verification

Packing and final verification are separate events.

Ready-for-shipment only after all required gates pass.

## Event history

Append-only events should include:
BOX_CREATED
CONTENTS_CHANGED
QC_STARTED
QC_COMPLETED
ITEM_REJECTED
WORK_REQUIRED
HANDOVER_SENT
HANDOVER_RECEIVED
REWORK_COMPLETED
PACKING_COMPLETED
FINAL_CHECK_COMPLETED
READY_FOR_SHIPMENT
BOX_SPLIT
BOX_MERGED
QUANTITY_CORRECTED

## Integrations

Returns:
returned item → Trace Box → QC/rework

Consignment:
ready Trace Box quantities may feed operational Consignment progress

Inventory:
Traceability does not directly mutate stock. Approved Inventory boundaries remain authoritative.

Printing:
QR/box labels may use canonical Printing Platform.

## Delivery

A — Trace Box foundation
B — QC + handover + work requirements
C — packing/final + Returns/Consignment integration
D — reporting
