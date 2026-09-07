# ADR-0005 — Single-business-first product direction

Status: Accepted by the owner’s explicit Phase 15 implementation request, 2026-09-07.
Supersedes the near-term SaaS product requirement in MASTER_SPEC sections 1, 2, 25, 26 and 40.

## Context

The existing platform already scopes records, sessions, permissions, foreign keys and
uniqueness by company. The operating priority is one internal business with multiple
marketplace seller identities, not commercial customer onboarding.

## Decision

Phases 15–25 are single-business-first. Defer customer signup, tenant switching, SaaS
billing, pricing and cross-customer administration. Preserve company_id, composite
constraints, server-established principals, query scoping, permissions and existing
module-entitlement behavior. They remain compatibility and safety boundaries.

Phase 15 changes documentation and package paths only. Single-business login/bootstrap
and operating-company selection belong to Phase 16; current login behavior is unchanged.
Seller/trading identities are separate from workstation/printer-agent identities.

## Alternatives and consequences

Removing company columns or weakening authorization would risk history and isolation;
continuing near-term SaaS development would distract from internal operational needs.
Retaining tested boundaries avoids both changes. Later commercial expansion requires a
new approved ADR. This decision does not claim the future operating model is implemented.
