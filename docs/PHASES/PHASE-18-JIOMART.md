# Phase 18 — JioMart

Status: BLOCKED_AWAITING_REPRESENTATIVE_EVIDENCE.
Revision: 2026-09-10.

## Goal and work

Build an isolated adapter after seller-account foundations using representative sanitized source evidence.

Phase 17 is locally verified and the owner has authorized Phase 18. An evidence review on
2026-09-10 classified the supplied private files as existing Flipkart, Snapdeal, Amazon and
Myntra material. None establishes a JioMart source format, field authority, page association
or printable layout. Implementation therefore remains blocked before adapter or contract
changes; no JioMart package has been created.

## Boundaries

Do not guess SKU, quantity, AWB, association or geometry. Missing/conflicting authoritative fields remain review-blocked.

## Acceptance and verification

Real fixtures, duplicate/retry/lease tests, account provenance, shared downstream regressions and review states.

The verified Phase 17 baseline was reproduced before implementation: a fresh disposable
PostgreSQL database was migrated through `000026`, and `make verify-full` passed with backend
formatting, vet, uncached PostgreSQL-backed tests, both executable builds, frontend typecheck,
lint, production build and repository checks. This is baseline evidence, not Phase 18
completion evidence. Private JioMart fixtures, JioMart parsing and JioMart printing remain
untested because representative JioMart input has not been supplied.

Every implementation plan must specify schema/API changes, owning modules and regression
coverage before editing. Stop after this phase; update CURRENT_STATE only with real evidence.
