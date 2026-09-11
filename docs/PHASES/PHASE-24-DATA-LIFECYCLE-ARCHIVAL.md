# Phase 24 — Data lifecycle and archival

Status: Active implementation — owner authorized 2026-09-11; completion requires recorded final verification.
Revision: 2026-09-11.

## Goal and work

Design protected database/object backup, export manifests, checksums, restoration and archival procedures.

The owning surface is the operator toolkit under `scripts/lifecycle`. It uses PostgreSQL's native
custom dump format and a byte-checked local object snapshot. It does not add a runtime service,
database table, HTTP route or frontend control.

## Boundaries

Demonstrate restoration before enabling deletion. Keep production backups and secrets separate from source exports; preserve audit history.

Phase 24 does not enable deletion. A matching verified restore receipt is mandatory before the
tool can produce an archival dry run. Audit logs, Inventory transactions and Trace Box events are
permanently excluded from object archival decisions. Production scheduling, provider-native S3
controls, retention approval and recovery objectives remain Phase 25 work.

## Acceptance and verification

Disposable database restore drill, object manifest verification, archival dry run and recovery evidence.

Acceptance also requires rejection tests for corrupt/traversing manifests, existing destinations,
unfinished or unsafe object sources, nonempty restore databases and receipts from a different
backup. The full project verification gate must run with PostgreSQL enabled.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
