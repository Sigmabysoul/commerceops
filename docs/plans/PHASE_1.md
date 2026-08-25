COMMERCEOPS — PHASE 1: CORE PLATFORM

ROLE

You are implementing Phase 1 of CommerceOps.

Phase 0 must already be approved and committed.

Do not modify foundational architecture unless explicitly authorized.


==================================================
MANDATORY READING
==================================================

Before implementation read:

- AGENTS.md
- docs/MASTER_SPEC.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/CURRENT_STATE.md
- docs/ROADMAP.md
- docs/phases/PHASE-01-CORE.md if present

Inspect the existing Phase 0 implementation before planning.


==================================================
PHASE GOAL
==================================================

Build the shared Core Platform used by all future CommerceOps modules.

Implement:

- Company/Tenant
- User identity foundation
- Employee
- Role
- Permission
- Role-Permission assignment
- User/Employee access association
- Module Entitlements
- Audit foundation for security/administrative changes

Do NOT implement marketplace, product, inventory or printing functionality.


==================================================
CRITICAL PRINCIPLE: MULTITENANCY
==================================================

CommerceOps is multi-company capable.

Company-owned data must be isolated.

Never trust a frontend-provided company_id as authorization.

Tenant/company context must be established server-side from authenticated context or another secure mechanism.

Cross-company access must be impossible through ordinary tenant APIs.

Write tests proving tenant isolation.


==================================================
COMPANY DOMAIN
==================================================

Implement a Company entity.

Initial fields may include concepts such as:

- id
- name
- status
- created_at
- updated_at

Do not prematurely add dozens of company settings.

Use stable IDs suitable for external references.

Company deletion must not casually destroy operational history.

If deletion is needed, prefer lifecycle/status concepts rather than destructive deletion.


==================================================
USER VS EMPLOYEE
==================================================

Keep these concepts separate.

User:
- authentication/login identity

Employee:
- worker/person participating in company operations

A User may correspond to an Employee, but they are not the same conceptual object.

Do not merge every employee field into the authentication user table.


==================================================
AUTHENTICATION
==================================================

Implement a simple, secure foundation appropriate for Phase 1.

Requirements:

- server-side authentication
- password hashes, never plaintext
- secure session/token approach
- logout
- user status
- company association/access

Do not build:

- Google OAuth
- social login
- enterprise SSO
- biometric login
- complex MFA

unless existing documentation explicitly requires it.

Prefer a simple system that can later be extended.


==================================================
EMPLOYEE DOMAIN
==================================================

Employees belong to a company.

Support basic concepts such as:

- id
- company_id
- display/name information
- active/inactive
- associated user account if applicable
- created_at
- updated_at

Do NOT hardcode the current workers into Go source code.

Initial employees must eventually be represented as database records/configuration.


==================================================
ROLE + PERMISSION MODEL
==================================================

Do not implement authorization as only:

admin
worker

Implement granular permissions.

Initial permission naming convention may include:

employees.view
employees.manage

roles.view
roles.manage

products.view
products.manage

labels.upload
labels.process
labels.print
labels.reprint

inventory.view
inventory.stock_in
inventory.adjust

returns.view
returns.process

consignment.view
consignment.create
consignment.edit
consignment.dispatch

reports.view

settings.manage


It is acceptable if permissions for future modules exist as seeds/configuration, but do not implement those modules.


==================================================
ROLE MODEL
==================================================

Roles are company-scoped collections of permissions.

Examples could later include:

Super Admin
Manager
Worker
Printing Operator
Inventory Manager

Do NOT hardcode business behavior based on role names.

Authorization must ultimately check permissions.

Role names are configuration.


==================================================
PLATFORM-LEVEL ACCESS
==================================================

If platform/system administration concepts are needed, keep them explicitly separate from normal tenant/company permissions.

Do not allow ordinary tenant administrators to gain cross-company access.


==================================================
MODULE ENTITLEMENTS
==================================================

Implement a foundation that determines which modules a company has access to.

Potential identifiers include:

core
flipkart
amazon
meesho
myntra
snapdeal
inventory
returns
consignment
advanced_reports

For Phase 1:

- implement entitlement storage and access checking
- Core should always be available as appropriate
- do NOT implement billing/payment logic
- do NOT hardcode ₹ pricing

Backend must enforce entitlements.

Frontend hiding alone is insufficient.


==================================================
AUDIT LOG FOUNDATION
==================================================

Create the shared audit mechanism.

Administrative/security events worth auditing may include:

- employee created
- employee disabled
- role created
- permissions changed
- user access changed
- module entitlement changed

Audit records should conceptually answer:

- company
- actor
- action
- target/entity
- timestamp
- relevant metadata

Do not create an overcomplicated event-sourcing system.


==================================================
API
==================================================

Create REST APIs needed for Phase 1.

Use:

/api/v1/...

Maintain existing API/error conventions.

Do not expose internal database structures unnecessarily.

Possible API areas:

/companies
/users
/employees
/roles
/permissions
/module-entitlements

Only expose operations actually required by the Phase.


==================================================
AUTHORIZATION MIDDLEWARE / SERVICE
==================================================

Create a centralized authorization mechanism.

Avoid scattered patterns like:

if user.role == "admin"

Use explicit permission checking.

Future modules must be able to reuse it.


==================================================
DATABASE
==================================================

Add migrations for Phase 1 schema.

Requirements:

- foreign keys
- useful indexes
- tenant/company ownership
- sensible uniqueness constraints
- timestamps
- no accidental cascading destruction of important historical data

Do not create future marketplace/product/inventory tables yet.


==================================================
FRONTEND
==================================================

Build only the administrative UI needed to exercise Phase 1.

Potential screens:

- login
- employee list
- create/edit employee
- roles
- permissions
- module access
- basic company settings

Keep UI functional and clean.

Do not build a huge polished dashboard.

Frontend must respect backend permission failures rather than assuming hidden UI equals security.


==================================================
TESTING REQUIREMENTS
==================================================

Tests must cover high-risk Core behavior.

At minimum:

1. tenant isolation
2. unauthorized user cannot access protected resource
3. permission allows authorized action
4. missing permission blocks action
5. disabled/inactive access behaves correctly
6. role changes affect authorization
7. module entitlement enforcement
8. password is never stored plaintext
9. administrative audit record creation where applicable

Use integration tests where database/authorization interaction matters.


==================================================
DO NOT IMPLEMENT
==================================================

Do not implement:

- Product Master
- SKU mapping
- Flipkart processor
- labels
- batches
- printing
- inventory
- returns
- consignment
- SaaS payment collection
- automated subscription charging


==================================================
DEFINITION OF DONE
==================================================

Phase 1 is implementation-complete when:

- Company domain works
- User/authentication foundation works
- Employee domain works
- roles work
- granular permissions work
- module entitlements work
- tenant isolation is enforced
- audit foundation works
- database migrations are valid
- frontend can manage relevant Core functions
- APIs are documented
- tests pass
- CI passes
- CURRENT_STATE.md is updated


==================================================
COMPLETION REPORT
==================================================

Report:

1. database migrations
2. domain entities implemented
3. APIs implemented
4. authorization design
5. tenant isolation strategy
6. tests executed
7. test results
8. security decisions
9. dependencies added
10. known limitations
11. documentation updates
12. architecture concerns

Finish with:

PHASE 1 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 1 NOT COMPLETE

STOP.
Do not begin Phase 2.