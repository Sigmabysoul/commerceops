# CommerceOps AI Engineering Rules

You are working inside the CommerceOps repository.

CommerceOps is a long-lived business operations platform. Treat existing architecture and business invariants as constraints, not suggestions.

## Required Reading

Before making architectural or business-logic changes, read:

* `docs/MASTER_SPEC.md`
* `docs/ARCHITECTURE.md`
* `docs/DOMAIN_RULES.md`
* `docs/CURRENT_STATE.md`

Read additional documentation relevant to the module being modified.

## Source of Truth

Priority for project decisions:

1. Explicit current user instruction
2. Approved architecture decision records
3. `docs/MASTER_SPEC.md`
4. `docs/ARCHITECTURE.md`
5. `docs/DOMAIN_RULES.md`
6. Module documentation
7. Existing implementation

If implementation conflicts with documented architecture, report the conflict instead of silently choosing one.

## Architecture

The system is a modular monolith.

Primary stack:

Frontend:
TypeScript + React + Next.js

Backend:
Go

Database:
PostgreSQL

File storage:
S3-compatible object storage

Specialized Python workers are allowed only for tasks where Python provides a significant ecosystem advantage.

Do not introduce a new primary framework, database, language or architectural style without explicit approval.

## Do Not Over-Engineer

Do not introduce technologies such as:

* microservices
* Kafka
* Kubernetes
* Redis
* Elasticsearch
* GraphQL
* additional databases
* message brokers

unless the task explicitly requires them or an approved ADR authorizes them.

Prefer the simplest design that satisfies current requirements.

## Module Ownership

Business domains own their logic.

Examples:

* inventory logic belongs to inventory
* return logic belongs to returns
* product normalization belongs to product
* marketplace parsing belongs to marketplace-specific processors
* printing logic belongs to printing
* authorization belongs to centralized authorization infrastructure

Do not bypass module boundaries for convenience.

## Business Logic

Do not place authoritative business logic in frontend components.

Do not place significant business logic directly in HTTP handlers.

Do not directly modify inventory outside the inventory domain.

Do not hardcode employees, company IDs, product IDs or marketplace-specific business assignments.

## Inventory Safety

Every meaningful inventory mutation must have an auditable reason.

Reprinting must never create another stock deduction.

Duplicate processing must not create duplicate inventory transactions.

Inventory operations must be transactional and idempotent where required.

## Multitenancy

Company/tenant isolation is mandatory.

Never expose data from one company to another.

Tenant context must be established server-side.

Never trust a frontend-provided `company_id` as authorization by itself.

## Database

All schema changes require migrations.

Do not manually mutate production schemas.

Avoid destructive schema migrations unless explicitly approved.

Preserve historical/audit data.

## APIs

Frontend/backend contracts must remain explicit.

Use documented REST/OpenAPI conventions.

Do not invent inconsistent response structures when an existing convention exists.

## Dependencies

Before adding a dependency:

1. check whether existing dependencies or the standard library can solve the problem
2. explain why the new dependency is necessary
3. avoid unnecessary large frameworks

Do not replace established dependencies casually.

## Code Changes

Prefer small, focused changes.

Do not rewrite unrelated functioning code.

Do not perform broad refactors unless requested.

Preserve compatibility unless a breaking change was explicitly authorized.

## Testing

Every bug fix should receive a regression test where reasonably possible.

Every important domain rule should be covered by tests.

Run applicable:

* unit tests
* integration tests
* lint
* formatting
* type checking
* build

before declaring work complete.

Never claim tests passed unless they were actually executed.

## Errors

Do not silently ignore errors.

Do not use broad error swallowing.

Operational failures must produce useful logs and appropriate user/system error states.

## Security

Never commit:

* passwords
* API keys
* access tokens
* private certificates
* database credentials

Validate all external input.

Treat PDFs and uploads as untrusted data.

Authorization must be enforced by the backend.

## Documentation

When behavior, architecture or a public contract changes, update the corresponding documentation.

Do not allow documentation and implementation to knowingly drift.

## Architecture Changes

If a requested implementation appears to require changing a foundational architectural decision:

STOP before performing the architecture change.

Explain:

* what conflicts
* why
* alternatives
* recommended solution

Architecture changes require explicit approval and an ADR.

## Task Discipline

For every task:

1. understand the requested outcome
2. locate the owning module
3. read applicable docs
4. inspect existing implementation
5. make the smallest correct change
6. test
7. report what changed
8. mention any remaining risks

Do not expand task scope without a concrete technical reason.

## Core Principle

Optimize for:

correctness
clarity
maintainability
traceability
low operational complexity

Not for:

cleverness
unnecessary abstraction
premature scalability
maximum number of technologies
