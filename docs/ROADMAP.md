# CommerceOps roadmap

Owner-approved planning direction, 2026-09-07.

## Implemented baseline

Phases 0–14 remain the historical baseline at `d90e4f7c0ceab032bc33a0618e1ceabdc20906b5` (migration 000022).
Myntra Phase 11 delivered CSV Batch A; print-payload completion remains pending.
Existing marketplace evidence limitations are preserved.

Phases 15–17 are complete. Phase 18 and the runtime portion of Phase 19 remain deferred on
their evidence gates. Phases 20–23 completed local PostgreSQL-backed verification on
2026-09-11. Phase 24 completed local PostgreSQL-backed verification. Phase 25 is active under
explicit owner authorization; production rollout and operator acceptance remain pending.

## Revised sequence

| Phase | Work |
|---|---|
| 15 | [Repository Architecture Consolidation](PHASES/PHASE-15-REPOSITORY-ARCHITECTURE-CONSOLIDATION.md) |
| 16 | [Single-business operating model and seller accounts](PHASES/PHASE-16-SINGLE-BUSINESS-SELLER-ACCOUNTS.md) |
| 17 | [Departments and Product ownership](PHASES/PHASE-17-DEPARTMENTS-PRODUCT-OWNERSHIP.md) |
| 18 | [JioMart](PHASES/PHASE-18-JIOMART.md) |
| 19 | [Myntra print-capture completion](PHASES/PHASE-19-MYNTRA-PRINT-CAPTURE-COMPLETION.md) |
| 20 | [Traceability foundation](PHASES/PHASE-20-TRACEABILITY-FOUNDATION.md) |
| 21 | [QC, rework, handover and packing](PHASES/PHASE-21-QC-REWORK-HANDOVER-PACKING.md) |
| 22 | [Consignment traceability integration](PHASES/PHASE-22-CONSIGNMENT-TRACEABILITY-INTEGRATION.md) |
| 23 | [Operations and workforce analytics](PHASES/PHASE-23-OPERATIONS-ANALYTICS.md) |
| 24 | [Data lifecycle and archival](PHASES/PHASE-24-DATA-LIFECYCLE-ARCHIVAL.md) |
| 25 | [Production hardening and internal rollout](PHASES/PHASE-25-PRODUCTION-HARDENING.md) |

## Deferred evidence backlog

- Phase 18 JioMart implementation is deferred until an original JioMart order export or
  shipping-label/invoice PDF establishes authoritative identifiers, quantity, seller-account
  provenance, association and any printable geometry. Supplied Flipkart, Snapdeal, Amazon and
  Myntra files must not be used to infer the JioMart contract.
- Phase 19 Myntra PDF parsing and print enrichment are deferred until the actual browser/OS
  print payload is captured. The production packed-orders CSV verifies Batch A but contains no
  quantity and cannot establish page association or geometry.

## Gates

CURRENT_STATE.md determines the active phase and records later explicit owner authorizations.
Documents for unstarted phases are planning, not implementation claims.
Single-business-first defers commercial SaaS while preserving company safety scope.
Production readiness is Phase 25; Phase 15 does not change login, accounts or operator UX.

Older Phase 15–19 drafts are retained under PHASES/SUPERSEDED with a mapping to this
sequence. Uncommitted seller-account code and migration 000023 are preserved separately
for reassessment in Phase 16. Never automatically start the next phase.
