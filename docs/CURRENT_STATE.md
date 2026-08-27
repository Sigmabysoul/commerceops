# CommerceOps Current State

## Current Phase

Phase 3 — Flipkart Processing

## Current Branch

`phase/03-flipkart`

## Approved Baseline

Phase 2 baseline: `d469f1800e4e277672b1c742da70985219495054`

## Phase Status

`IMPLEMENTATION_IN_PROGRESS`

Phase 3 implementation and automated remediation checks exist, but Phase 3 is
not approved or complete. External architecture review remains outstanding.

Implemented Phase 3 facts include:

- tenant-owned PDF upload with a 20 MiB limit and per-company SHA-256 source
  deduplication
- PostgreSQL-backed, bounded Flipkart processing jobs and scoped recovery
- platform object-storage abstraction with local development and production
  S3-compatible implementations
- bounded Poppler extraction and isolated `flipkart-text-v2` parsing with real
  source-page traceability
- AWB, order ID, SKU, and explicit quantity extraction
- exact Product Master resolution with unresolved/manual-review states
- duplicate identifier protection, visible processing errors, retry support,
  authorization, entitlements, audit events, and a functional processing UI

## Approved Phases

- Phase 0 — Foundation
- Phase 1 — Core Platform
- Phase 2 — Product Master

## Blocking Issues

- [ ] Validate representative production Flipkart PDF formats; only sanitized
  fixtures have been validated.
- [ ] Replace the documented single-instance recovery limitation with worker
  leases before multi-instance deployment.
- [ ] Pass external architecture review.

## Explicitly Forbidden Work

- Phase 4 batch orchestration or printing
- inventory mutations or inventory functionality
- Amazon, Meesho, Myntra, Snapdeal, or other marketplace implementation
- returns, cancellations, consignment, or printer-agent implementation
- automatic progression to another phase

## Last Verification

The latest recorded Phase 3 verification passed:

- migration up against disposable PostgreSQL 18.6
- full Go suite, including PostgreSQL tenant, duplicate, retry, and recovery
  integration tests, storage-driver validation, and S3-compatible
  Put/Get/Delete protocol tests
- Go vet and Go build
- frontend type checking, lint, and production build
- `git diff --check`

Verification must be rerun after subsequent implementation changes. Historical
results must not be presented as proof that a newer working tree passes.

## Next Allowed Task

Continue Phase 3 review, fixture validation, reliability remediation,
documentation, or developer-experience work explicitly authorized by the
owner. Do not begin Phase 4 until Phase 3 passes its review gate and the owner
authorizes the transition.
