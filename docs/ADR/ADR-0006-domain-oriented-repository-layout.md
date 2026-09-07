# ADR-0006 — Domain-oriented repository layout

Status: Accepted by the owner’s explicit Phase 15 implementation request, 2026-09-07.

## Context

The Go backend mixes domain and technical packages directly under internal. Clearer
navigation is useful without changing the modular-monolith architecture or ownership.

## Decision

Retain one Go module, executable locations under cmd, and the existing frontend.
Use internal/app for composition, internal/domain for business packages and
internal/platform for technical mechanisms. Keep the scheduler in Automation and
marketplace orchestration in the marketplace root package. Preserve separate PDF
extractor and generator packages under platform/documents/pdf and their package names.

Do not create empty future packages, new layering frameworks or shared business-rule
packages. Tests and fixtures stay colocated. Internal import paths may change; API,
schema, authentication, permissions, state transitions and dependencies may not.

## Alternatives and consequences

Keeping the flat layout preserves navigation friction. Broad clean-architecture layers
or splitting services would add complexity. Mechanical package moves preserve the
existing dependency graph; relative fixture/migration paths and both executables require
verification. Each coherent move is tested and committed separately for rollback.
