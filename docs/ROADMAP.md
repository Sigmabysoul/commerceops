# CommerceOps Roadmap

Phase documents describe intended scope. Only approved phases and the current
phase are actionable; locked future phases are design references and may not be
implemented until the current review gate passes and the owner authorizes the
transition.

## Approved and completed

- **Phase 0 — Foundation:** repository, runtime, PostgreSQL, migrations,
  frontend, CI, and architecture foundation.
- **Phase 1 — Core Platform:** companies, identities, employees, roles,
  permissions, entitlements, sessions, and audit foundation.
- **Phase 2 — Product Master:** canonical products, marketplace references, SKU
  mappings, and deterministic training/resolution.

## Current phase

- **Phase 3 — Flipkart Processing:** implementation and review in progress.
  Secure uploads, extraction, normalization, Product Master resolution,
  duplicates, jobs, manual review, and UI are within scope. Inventory, batches,
  and printing are not.

Phase 3 must pass its documented automated checks, representative fixture
validation, and external architecture review before any phase transition.

## Locked future phases

- Phase 4 — Batch and printing
- Phase 5 — Inventory
- Phase 6 — Dashboard and reporting
- Phase 7 — Amazon
- Phase 8 — Returns and cancellations
- Phase 9 — Consignment management
- Phase 10 — Meesho
- Phase 11 — Myntra
- Phase 12 — Snapdeal
- Phase 13 — Printer agent
- Phase 14 — Advanced automation

The sequence can change only through an explicit product/architecture decision.
No AI agent may automatically start a locked phase.
