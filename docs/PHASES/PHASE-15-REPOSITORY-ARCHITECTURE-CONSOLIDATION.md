# Phase 15 — Repository Architecture Consolidation

Status: Active implementation — owner approved all five structural batches.
Revision: 2026-09-07.

## Goal and work

Move existing packages under app/domain/platform in five verified batches; preserve package names and domain ownership.

## Boundaries

No runtime, API, auth, permission, company-scope, dependency or schema changes. No new migrations or feature packages.

## Acceptance and verification

Full PostgreSQL-backed verify-full; both executables; migration/contract checksums; stale import and fixture-path review.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.

## Approved implementation sequence

1. Preserve source WIP on a separate ref with a temporary index; leave both original checkouts intact.
2. Verify the full-history bundle and exact Phase 14 source archive; use commerceops-next.
3. Reconcile planning documents/ADRs in a separate commit; retain historical evidence.
4. Run a fresh full baseline gate on a disposable database migrated through 000022.
5. Follow CODEX/MIGRATION_MAP.md batches 1–5. Update every direct importer and affected relative path; verify and commit each batch.
6. Build server and printer-agent in backend CI. Run final full verification and checksums.
7. Record evidence, known skips and a clean committed working tree. Do not start Phase 16.

Schema/API impact: NONE. Risks: relative migration/fixture paths, changed import depth,
accidental package merging and loss of historical documentation. No app/scheduler split.

Rollback: revert the affected batch commit on the isolated branch, or restore a new clone
from the verified bundle. Do not reset the original seller-account checkout.
