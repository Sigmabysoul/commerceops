# CommerceOps — Master Project Specification

**Version:** 0.2
**Status:** Owner-approved planning direction for Phases 15–25
**Architecture Status:** Approved foundation
**Project Type:** Internal ecommerce and warehouse operations platform

---

# 1. Vision

CommerceOps is an internal ecommerce and warehouse operations platform for the current business. Reliability, traceability and safe inventory accounting take priority. Commercial multi-customer SaaS is deferred beyond Phases 15–25 under ADR-0005.

The system will gradually manage:

* Ecommerce label processing
* Ecommerce order processing
* Label cropping and modification
* Batch creation
* Product identification
* SKU normalization
* Printing
* Inventory
* Stock in/out
* Returns
* Cancellations
* Consignment orders
* Stock reservations
* Employees
* Worker assignments
* Roles and permissions
* Audit logs
* Reporting
* Existing module entitlements (compatibility boundary)

Supported ecommerce marketplaces are planned to include:

* Flipkart
* Amazon
* Meesho
* Myntra
* Snapdeal

Additional marketplaces must be addable later without redesigning the core system.

---

# 2. Product strategy

Serve one operating business through Phases 15–25. Retain company ownership and existing
module entitlements internally; do not build customer-company signup, tenant switching,
subscription billing, SaaS pricing or cross-customer administration in these phases.

The roadmap adds seller accounts, product department history, JioMart, Myntra print
completion, Trace Boxes, QC/rework/handovers/packing, Consignment traceability, analytics,
backup/restore/archive and production hardening. These are future capabilities until
implemented and verified under their individual phase gates.

Exactly one active operational department per Product is the Phase 17 target. Reuse
existing departments; assignment changes affect future routing while preserving historical
and in-flight context. Seller accounts belong to business/trading identities and must
remain separate from workstation/printer-agent identity.

---

# 3. Architecture philosophy

CommerceOps will begin as a **modular monolith**.

It must NOT begin as microservices.

Initially:

* One main Go backend
* One PostgreSQL database
* One Next.js frontend
* One object-storage system
* Optional specialized workers

Each business domain must have clearly separated boundaries.

Examples:

* inventory
* products
* labels
* batches
* returns
* consignment
* employees

Modules should communicate through defined domain interfaces rather than directly modifying each other's internal data.

A module may later be extracted into a separate service if there is a demonstrated scaling requirement.

---

# 4. Technology stack

## Frontend

Language:

TypeScript

Framework:

Next.js + React

Purpose:

* dashboards
* forms
* operational screens
* administration
* reporting
* responsive mobile UI
* PWA functionality

Plain JavaScript should not be used for application code.

Strict TypeScript should be enabled.

---

## Backend

Language:

Go

Purpose:

* API
* business logic
* authentication
* permissions
* inventory calculations
* ecommerce processing orchestration
* batch processing
* reports
* background jobs
* existing module-entitlement enforcement
* audit logging

Go is the primary server-side application language.

---

## Database

PostgreSQL.

PostgreSQL is the primary source of structured business data.

---

## File Storage

Large binary files such as PDFs must normally be stored using S3-compatible object storage.

The database should store metadata and storage references rather than large PDFs directly.

---

## Python

Python is NOT a second general backend.

Python may be used only when a specialized library provides a meaningful advantage, such as:

* OCR
* computer vision
* machine learning
* unusual PDF processing
* experimental document extraction

Basic business logic must remain in Go.

---

## Desktop Software

The primary application is web-based.

A small local desktop/printing agent may be developed later for:

* automatic printing
* printer selection
* print queues
* printer status
* local hardware access
* barcode printers
* thermal printers

The entire CommerceOps application must not depend on desktop installation.

---

# 5. Company safety boundary

Existing company_id columns, company-scoped uniqueness, composite foreign keys and
server-established authenticated company context remain mandatory. No domain may
trust a frontend company UUID as authorization. Isolation tests remain required.

Single-business-first changes product direction, not Phase 15 authentication behavior.
Phase 16 will implement normal login without requiring raw company UUID entry, using an
explicit server-side operating-company policy. Do not guess company identity or hardcode
business identifiers. Preserve current login, permissions and module entitlements during
Phase 15.

---

# 6. Core Platform

The Core Platform contains shared infrastructure used by all modules.

Core responsibilities include:

* Company
* User
* Employee
* Authentication
* Role
* Permission
* Product Master
* SKU Mapping
* Module Entitlements
* Audit Logs
* Application Settings

Other modules must reuse these shared concepts.

They must not independently recreate users, employees, products, or permissions.

---

# 7. Product Master

CommerceOps must use an internal Product Master.

Marketplace SKU strings are not considered canonical product identities.

Example internal product:

Product:
Garbage Bag

Brand:
Averx

Variant:
3 Bag

Internal Code:
GB-AVX-3B

Marketplace aliases may include multiple different SKU strings.

Example:

Flipkart SKU → GB-AVX-3B

Amazon SKU → GB-AVX-3B

Meesho SKU → GB-AVX-3B

All marketplaces therefore refer to the same internal product.

---

# 8. Product Training

Unknown SKUs must be trainable.

When an unknown SKU appears, an authorized worker should be able to:

1. select the internal product
2. optionally configure parsing information
3. optionally configure quantity interpretation
4. assign worker rules if relevant
5. save the mapping

Future occurrences should be recognized automatically.

Training results must remain editable.

The system should maintain information about who created or changed a mapping.

---

# 9. Ecommerce Architecture

Marketplace-specific parsing must remain isolated.

Conceptually:

Marketplace Processor
→ Normalized Order
→ CommerceOps Core

Marketplace processors include:

* FlipkartProcessor
* AmazonProcessor
* MeeshoProcessor
* MyntraProcessor
* SnapdealProcessor

The rest of CommerceOps should not need to understand marketplace-specific PDF structures.

Each processor converts marketplace information into a normalized internal order representation.

---

# 10. Normalized Order

A normalized ecommerce order should be capable of representing:

* company
* marketplace
* marketplace order ID
* AWB/tracking identifier
* order date
* label
* products
* quantities
* marketplace SKU
* internal product
* processing status
* printing status
* cancellation state
* return state

The exact database implementation will be specified separately.

---

# 11. Ecommerce Processing Pipeline

Target workflow:

File received/uploaded

→ identify marketplace

→ split/crop documents where required

→ extract order information

→ extract AWB/tracking identifier

→ extract marketplace SKU

→ resolve internal product

→ determine quantity

→ detect duplicates

→ assign employee

→ group into batch

→ calculate batch quantities

→ generate print-ready labels

→ send to print queue

→ track print status

→ update downstream stock workflow

Every significant failure must produce a visible error state rather than silently skipping data.

---

# 12. Batch System

Orders and labels must be grouped into processing batches.

A batch should allow the system to track:

* marketplace
* creation time
* source file
* number of labels
* number of orders
* product quantities
* employee assignments
* processing progress
* processing errors
* printing progress
* completion state

Batch states should be explicit.

Example concept:

RECEIVED
PROCESSING
READY
PRINTING
COMPLETED
FAILED
CANCELLED

Exact state machines will be documented later.

---

# 13. Background Processing

Large document processing must not depend on a long-running HTTP request.

Expected model:

Upload

→ create processing job

→ return job identifier

→ background worker processes data

→ progress stored

→ frontend polls/subscribes to progress

→ processing completes

This prevents large batches from failing due to HTTP timeouts.

Concurrency must be bounded.

The system must never create unlimited goroutines or workers based directly on user input.

---

# 14. Duplicate Protection

Where marketplaces provide reliable unique identifiers such as AWB or order ID, those identifiers must be used to detect duplicate processing.

Duplicate uploads should not silently generate duplicate stock movements.

Operations affecting inventory should be idempotent where possible.

Reprinting a label must not automatically create another stock deduction.

---

# 15. Printing

Printing is its own business domain.

CommerceOps must differentiate:

* processed
* ready for printing
* print requested
* printing
* printed
* print failed
* reprinted

Reprinting must remain traceable.

A future printer agent may process print jobs created by the central platform.

---

# 16. Inventory

CommerceOps will have one centralized inventory system.

Ecommerce, returns, cancellations and consignment must use the same inventory domain.

Inventory must NOT be implemented as unrelated counters owned independently by each module.

---

# 17. Inventory Ledger

Important inventory changes must create inventory transactions.

Example transaction:

Product:
Averx Garbage Bag 3B

Quantity:
-18

Reason:
Ecommerce dispatch

Reference:
Flipkart Batch XYZ

Actor:
System / Employee

Timestamp:
...

The system must preserve historical transaction information.

Never modify stock without an auditable reason.

---

# 18. Inventory Transaction Types

Potential transaction categories include:

STOCK_IN
ECOMMERCE_OUT
CONSIGNMENT_OUT
RETURN_RESTOCK
CANCELLATION_RESTOCK
DAMAGED
MANUAL_ADJUSTMENT
CORRECTION

The final list will be defined in DOMAIN_RULES.md.

---

# 19. Stock Reservations

Consignment orders may require inventory before dispatch.

CommerceOps should support:

Physical Stock
Reserved Stock
Available Stock

Concept:

Available = Physical - Reserved

Creating a consignment may reserve stock.

Actual inventory deduction occurs according to the approved dispatch workflow.

---

# 20. Returns

Returns must link back to the original order whenever possible.

Return workflow should support states such as:

Expected
Received
Inspection Pending
Restocked
Damaged
Wrong Product
Missing
Rejected
Resolved

A returned item classified as usable/restockable may create a positive inventory transaction.

A damaged return must not automatically become sellable inventory.

---

# 21. Cancellation Management

Cancellations are separate from physical returns.

The system must differentiate cases such as:

* cancelled before stock deduction
* cancelled after printing
* cancelled before dispatch
* cancelled after dispatch

Inventory behavior depends on the actual operational state.

No duplicate stock restoration should occur.

---

# 22. Consignment Management

The Consignment module will eventually replace the company's Google Sheet workflow.

It should eventually support:

* scheduled dispatch date
* consignment files
* products
* quantities
* required inventory
* stock reservation
* shortage warnings
* assigned employees
* dispatch status
* box planning
* loading
* completion
* historical records

Future functionality may include:

* box calculation
* dimensional calculations
* truck planning
* dealer management
* loading checklists
* document generation

---

# 23. Employee Assignment System

Product and marketplace responsibility must NOT be hardcoded into application code.

CommerceOps must support configurable assignment rules.

Initial business configuration:

## Default worker

Sohel handles ecommerce services generally and products not assigned to another worker.

## Kartik

Kartik currently handles:

Garbage Bags:

* 2 Bag
* 3 Bag
* Averx
* Star
* Plain

Garbage Rolls:

* 17x19
* 19x21

Butter Paper

Aluminium Containers:

* 25 pc
* 50 pc
* occasionally 100 pc

These are initial configuration values, NOT permanent source-code rules.

Future employees must be assignable without changing Go code.

---

# 24. Roles and Permissions

Permissions must be granular.

Potential permissions include:

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

employees.view
employees.manage

reports.view

settings.manage

Roles are collections of permissions.

Do not implement authorization solely as:

if admin

or:

if worker

---

# 25. Module entitlements

Existing backend-enforced module entitlements remain part of the compatibility baseline.
They do not require building commercial subscriptions or new multi-customer administration.
Frontend visibility is never a replacement for server authorization.

---

# 26. Commercial strategy

SaaS pricing, billing and customer subscription products are outside Phases 15–25.
Any later commercial expansion requires an approved product decision and ADR; prices
and business identities must never be hardcoded into the architecture.

---

# 27. Audit System

Important actions must be auditable.

Audit records should answer:

Who?
What?
When?
Which company?
Which entity?
What changed?
Why?

Examples:

* stock adjustment
* SKU mapping changed
* label reprinted
* role changed
* return restocked
* consignment dispatched

Critical audit history should not be silently rewritten.

---

# 28. Reporting

The dashboard should eventually support daily and historical reporting.

Examples:

Labels per marketplace
Orders per marketplace
Processing failures
Batch counts
Product quantities
Stock in
Stock out
Net stock movement
Employee workload
Returns
Cancellations
Consignments
Inventory shortages
Printing failures

Reports must derive from authoritative domain data rather than maintaining unrelated manual counters whenever possible.

---

# 29. Security

CommerceOps will contain operational business information.

Minimum expectations include:

* no secrets committed to Git
* hashed passwords
* authorization checked server-side
* tenant isolation
* validation of uploaded files
* upload size limits
* safe file names
* rate limiting where appropriate
* audit logging
* database backups
* least-privilege access

Security implementation details belong in SECURITY.md.

---

# 30. API Architecture

Frontend and backend communicate through an explicit API.

REST is the initial API style.

OpenAPI should describe the API contract.

TypeScript API clients/types may be generated from the contract.

Frontend code should not guess backend response shapes.

Versioned API route concept:

/api/v1/...

---

# 31. Codebase Boundaries

Business logic must not live primarily inside HTTP handlers.

Database queries must not be scattered randomly through the application.

Marketplace-specific parsing must remain inside marketplace-specific modules.

Inventory mutations must pass through the inventory domain.

Authorization must pass through centralized authorization mechanisms.

Frontend UI components must not contain authoritative inventory or accounting business logic.

---

# 32. Performance Philosophy

CommerceOps should prefer efficient and predictable designs.

However:

Do not prematurely optimize.

Do not introduce distributed infrastructure without measured need.

Do not add:

* Kubernetes
* Kafka
* microservices
* Elasticsearch
* Redis
* message brokers

unless a documented requirement justifies them.

Begin simply and measure.

---

# 33. Development Philosophy

Reliability and traceability take priority over flashy automation.

Each feature should be implemented incrementally.

Before modifying code:

1. understand the affected module
2. read applicable documentation
3. define expected behavior
4. implement the smallest correct change
5. run tests
6. verify architecture boundaries
7. update documentation if behavior changed

---

# 34. AI Development Policy

AI coding agents are contributors, not architects with unlimited authority.

They must follow the project's documented architecture.

An AI must not independently:

* replace the backend language
* replace PostgreSQL
* introduce microservices
* add major infrastructure
* move responsibilities between domains
* redesign authentication
* redesign multitenancy
* introduce a second business backend
* rewrite functioning modules
* remove audit behavior
* change inventory accounting rules

Such changes require an explicit architecture decision.

---

# 35. Architecture Decision Records

Major technical decisions must be documented using ADR files.

Example:

ADR-0001-modular-monolith.md

An ADR records:

Context
Decision
Reasoning
Consequences
Alternatives considered

Approved architectural decisions remain authoritative until superseded by another approved ADR.

---

# 36. Initial Development Sequence

Phase 0 — Project Foundation

Phase 1 — Core Platform

Phase 2 — Product Master

Phase 3 — Flipkart processing

Phase 4 — Batch and printing system

Phase 5 — Inventory

Phase 6 — Dashboard/reporting

Phase 7 — Amazon

Phase 8 — Returns and cancellations

Phase 9 — Consignment management

Phase 10 — Meesho

Phase 11 — Myntra

Phase 12 — Snapdeal

Phase 13 — Printer agent

Phase 14 — Advanced automation

Sequence may change based on real operational findings, but architectural foundations should not.

---

# 37. Phase 0 Scope

Before business features are written, establish:

Repository structure
Documentation structure
Go project
Next.js TypeScript project
PostgreSQL local environment
Migration system
Testing structure
Linting
Formatting
CI
Environment configuration
Logging conventions
Error conventions
OpenAPI strategy
Architecture rules

Phase 0 is considered complete only when a clean test project can build and CI can validate it.

---

# 38. Non-Goals for Initial Versions

Do not initially build:

* microservices
* native Android application
* full offline synchronization
* automatic scraping of every marketplace
* advanced AI product recognition
* complex subscription payment processing
* Kubernetes infrastructure
* multi-region deployment
* enterprise analytics
* automatic printer control

These may be added when actual requirements justify them.

---

# 39. Primary Design Principle

CommerceOps must remain easy to understand.

A new developer or AI agent should be able to identify:

* which module owns a feature
* where business logic belongs
* where database logic belongs
* what rules must never be violated
* how modules communicate
* how to test a change

Clarity is preferred over cleverness.

---

# 40. Long-term goal and roadmap authority

Grow a reliable internal operations platform without rewriting its foundations.
Commercial expansion can be reconsidered after internal maturity; it is not an active
Phase 15–25 requirement. Phases 0–14 remain the historical implementation baseline.
See ROADMAP.md for sequencing and CURRENT_STATE.md for the active implementation gate.

Phase 15 moves packages under internal/app, internal/domain and internal/platform only.
It does not change APIs, schema, migrations, authentication, permissions, company scope,
marketplace behavior, stock rules or frontend behavior. ADR-0006 defines the layout.

Trace identifiers in later phases must be opaque server-generated values; mutable state
stays in PostgreSQL. Traceability never duplicates inventory balances. QC PASS is not
RESTOCK; only an explicit authorized Inventory transition changes sellable stock.

Trace Box QC covers the complete current content snapshot. Rejected quantity creates explicit
rework, and completed work requires a fresh passing QC before packing. Handovers remain in transit
until the target employee or a member of the target department records receipt. Packing, final
verification and shipment readiness are distinct audited gates owned by Traceability.

Consignments may opt into verified Trace Box evidence. In that mode, each line's ready and packed
quantity must be covered by currently shipment-ready Trace Box quantity, and ready, packed and
outbound gates revalidate complete coverage. Allocations preserve the line's Product and department
snapshot, cannot double-count physical quantity across consignments, and retain immutable reversals.
Pouch and file evidence references are auditable but are not unique identities.

This document remains the product-level source of truth, subject to explicit owner
instructions and approved ADRs.

---
