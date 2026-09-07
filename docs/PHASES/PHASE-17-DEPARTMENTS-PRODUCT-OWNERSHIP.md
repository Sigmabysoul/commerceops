# Phase 17 — Departments and Product ownership

Status: Future planning only — requires preceding verification and explicit owner authorization.
Revision: 2026-09-07.

## Goal and work

Reuse canonical departments from Consignment. Add effective-dated Product assignment/history and future routing.

## Boundaries

Exactly one active department per Product; authorized concurrent switches preserve historical and in-flight context. Do not create a second department table.

## Acceptance and verification

Constraints, concurrent reassignment, permissions/audit, routing and historical snapshots.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
