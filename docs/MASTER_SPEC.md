# CommerceOps — Master Project Specification

**Version:** 0.1
**Status:** Planning
**Architecture Status:** Approved foundation
**Project Type:** Modular ecommerce operations platform / future SaaS

---

# 1. Vision

CommerceOps is a modular business operations ecosystem intended initially for one ecommerce/warehouse company and designed from the beginning so it can later become a multi-company SaaS platform.

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
* Subscription modules

Supported ecommerce marketplaces are planned to include:

* Flipkart
* Amazon
* Meesho
* Myntra
* Snapdeal

Additional marketplaces must be addable later without redesigning the core system.

---

# 2. Product strategy

CommerceOps will first be developed and tested as an internal company application.

The architecture must nevertheless support a future SaaS model.

Each customer/company must have logically isolated data.

Companies may subscribe to different CommerceOps modules.

Example:

Company A:

* Core
* Flipkart
* Inventory

Company B:

* Core
* Flipkart
* Amazon
* Returns

Company C:

* Complete system

Pricing is NOT part of the permanent architecture.

Prices may change without changing software architecture.

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
* subscriptions
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

# 5. Multitenancy

CommerceOps must be designed as multi-company capable from the beginning.

Primary business entities should normally have ownership through a company/tenant.

Example:

company_id

Data belonging to one company must never accidentally be visible to another company.

Tenant filtering must be handled systematically rather than relying on developers remembering to add filters manually.

Cross-company data access is forbidden unless performed by an explicitly authorized platform-level operation.

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

# 25. SaaS Modules

CommerceOps should support module entitlements.

Potential modules:

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

A company's enabled modules determine available functionality.

Module access must be enforced by the backend, not merely hidden in the frontend.

---

# 26. Subscription Strategy

Pricing is intentionally separated from technical modules.

Example commercial concepts may later include:

* base platform fee
* marketplace modules
* operational modules
* storage tiers
* user limits
* usage limits
* complete-package pricing

Specific prices are business decisions and must not be hardcoded into architecture.

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

# 40. Long-Term Goal

CommerceOps should be capable of growing from:

one company
two workers
a few marketplace workflows

into:

multiple companies
many employees
many marketplaces
large inventory
multiple warehouses
automated printing
returns
consignment planning
subscription modules
business analytics

without requiring the application to be rewritten from scratch.

This document is the product-level source of truth.

Changes to fundamental architecture must be intentional, documented and approved.
