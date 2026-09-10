# Phase 20 — Traceability foundation

Status: ACTIVE — owner authorized implementation on 2026-09-11.
Revision: 2026-09-11.

## Goal and work

Introduce Trace Boxes, opaque identifiers, contents/custody relationships and append-only operational events.

## Implementation contract

The Traceability domain owns Trace Boxes and their history. A Trace Box receives a
cryptographically random, globally unique opaque identifier. Labels and scans carry only that
identifier; resolving it always requires an authenticated company-scoped API request.

Box contents reference canonical Product Master records and explicit positive quantities.
Adding and removing quantities creates immutable, idempotent content events. Current contents
are derived from the signed event quantities, and removal cannot exceed the current quantity.

Custody changes reference either one canonical employee or one canonical department in the
same company. Every transfer is an immutable, idempotent event; current custody is the latest
event. This phase does not introduce QC state, rework, handover acceptance, packing gates,
Returns integration, or Consignment integration.

The domain exposes company-scoped REST endpoints for listing, creating, resolving and reading
Trace Boxes, and for content and custody changes. Reads require `traceability.view`; mutations
require `traceability.manage`. The `traceability` module entitlement gates both permissions.

Schema changes: migration `000027_traceability_foundation` adds Trace Boxes, immutable unified
events, event-linked content changes, event-linked custody changes, permissions and indexes.
It does not alter Inventory tables or create a second stock balance.

## Boundaries

QR/barcodes contain identifiers only. No duplicated Inventory balance or implicit stock movement.

## Acceptance and verification

Identifier resolution, permissions/company isolation, replay/idempotency, bounded contents,
custody history, append-only database enforcement, migration round-trip, API routing and a
small authenticated operator workspace.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
