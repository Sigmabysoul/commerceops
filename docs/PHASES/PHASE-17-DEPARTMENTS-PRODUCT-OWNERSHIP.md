# Phase 17 — Departments and Product ownership

Status: In progress — owner authorized implementation after verified Phase 16.
Revision: 2026-09-10.

## Goal and work

Reuse canonical departments from Consignment. Add effective-dated Product assignment/history and future routing.

Implementation records authorized Product reassignment history, permits at most one active
department, and requires new Consignment lines to use that current assignment. Existing lines
remain unchanged so historical and in-flight routing is preserved.

## Boundaries

Exactly one active department per Product; authorized concurrent switches preserve historical and in-flight context. Do not create a second department table.

## Acceptance and verification

Constraints, concurrent reassignment, permissions/audit, routing and historical snapshots.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
