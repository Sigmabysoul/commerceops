# Phase 17 — Departments and Product ownership

Status: IMPLEMENTATION_COMPLETE_LOCAL_VERIFIED on 2026-09-10.
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

Verification evidence: migrations through `000026` applied to a fresh disposable PostgreSQL
database; `000026` also passed down/up round-trip verification. PostgreSQL-backed Product and
Consignment regression tests cover tenant scope, permissions, concurrent reassignment, one
active assignment, routing, and preserved line snapshots. `make verify-full` passed with
backend formatting, vet, uncached tests, both executable builds, frontend typecheck, lint,
production build, and repository checks. Private fixtures, hardware tests, and remote CI were
not run for this branch.
