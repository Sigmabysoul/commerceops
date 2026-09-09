# Phase 16 — Single-business operating model and seller accounts

Status: In progress. Owner authorized conditional progression on 2026-09-09 after Phase 15 verification.
Revision: 2026-09-09.

## Goal and work

Implement operating-company login/bootstrap and business/trading identities with marketplace seller accounts. Reassess the preserved WIP rather than cherry-picking it blindly. Keep account provenance, dedupe and SKU mappings explicit.

Implemented work: login selects the sole active company access server-side and rejects
ambiguous access; business identities and seller accounts are company-scoped and permission
protected; marketplace ingestion, SKU mappings, normalized orders, batches, returns,
cancellations, and order documents retain seller-account provenance. Historical rows are
backfilled to dormant unassigned-legacy accounts rather than attributed to a seller.

## Boundaries

Keep company_id and server-established scope. Never infer account ownership from workstation/printer identity. Zero or multiple operating-company accesses require a defined server policy before implementation.

## Acceptance and verification

Auth edge cases, permissions, company isolation, account-aware mappings/deduplication/provenance, migrations and OpenAPI.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
