# Module Ownership

CommerceOps is a modular monolith. Ownership describes responsibility boundaries
inside one application; it does not imply microservices.

## Authentication (`internal/platform/auth`)

- **Owns:** login identity verification, password hashing, session creation,
  authentication cookies, logout, and authenticated principal construction.
- **Does not own:** roles, permissions, company business data, or marketplace
  processing.
- **Allowed dependencies:** PostgreSQL platform access and shared HTTP response
  utilities.
- **Forbidden leakage:** handlers or other domains must not construct trusted
  tenant principals from request-provided `company_id` values.

## Authorization (`internal/platform/authorization`)

- **Owns:** centralized permission and module-entitlement decisions for an
  authenticated principal.
- **Does not own:** authentication, role administration UI, or business-domain
  rules.
- **Allowed dependencies:** authenticated principals and tenant-scoped role,
  permission, and entitlement records.
- **Forbidden leakage:** domains and frontend components must not replace
  authorization checks with role-name checks or UI visibility.

## Core and company (`internal/domain/core`)

- **Owns:** companies, employees, company user access, roles, permission
  assignments, module-entitlement administration, and audit-log retrieval.
- **Does not own:** authentication internals, canonical products, marketplace
  parsing, inventory, or printing.
- **Allowed dependencies:** authentication principals, centralized
  authorization, audit recording, and PostgreSQL.
- **Forbidden leakage:** company ownership must not be inferred from frontend
  input, and employee assignments must not be hardcoded in other modules.

## Product Master (`internal/domain/product`)

- **Owns:** canonical tenant products, lifecycle state, effective-dated department
  ownership, marketplace SKU mappings, deterministic exact resolution, and Product Master training.
- **Does not own:** marketplace document parsing, order ingestion, inventory
  balances, or worker assignment.
- **Allowed dependencies:** canonical Consignment department identities, authenticated
  principals, authorization, audit, and normalized marketplace keys.
- **Forbidden leakage:** marketplace SKU strings must never become canonical
  product identity, and marketplace processors must not invent products.

## Marketplace (`internal/domain/marketplace`)

- **Owns:** marketplace upload orchestration, source/job metadata, normalized
  marketplace orders/items, duplicate detection, processing states, review
  errors, database-backed job leases, and marketplace-specific adapters for
  Flipkart, Amazon, Meesho, Myntra, and Snapdeal.
- **Does not own:** canonical product definitions, authentication,
  authorization policy, object persistence mechanics, inventory, batches, or
  printing.
- **Allowed dependencies:** Product Master resolution, authorization, audit,
  PostgreSQL, object-storage interfaces, and PDF-extraction interfaces.
- **Forbidden leakage:** Flipkart, Amazon, Meesho, Myntra, and Snapdeal parsing rules must remain in their
  isolated adapters; marketplace processing must not mutate inventory or
  implement future marketplace adapters outside the active phase.

## Marketplace seller accounts (`internal/domain/marketplaceaccount`)

- **Owns:** company-scoped business/trading identities and marketplace seller-account
  configuration.
- **Does not own:** workstation or printer-agent identity, parsing, Product Master records,
  inventory, or hardware routing.
- **Allowed dependencies:** authenticated principals, centralized authorization, audit, and
  PostgreSQL.
- **Forbidden leakage:** account ownership must be supplied explicitly by the approved
  marketplace workflow; it must never be inferred from a workstation, SKU, filename, or
  marketplace-only fallback.

## Inventory (`internal/domain/inventory`)

- **Owns:** stock ledger transactions, balances, source-linked reservations,
  ready-batch ecommerce outbound confirmation, adjustments, corrections, and
  inventory idempotency.
- **Does not own:** marketplace parsing, returns disposition, printing, or
  product identity.
- **Allowed dependencies:** Product Master identities, authenticated actors,
  authorization, audit, and approved domain references.
- **Forbidden leakage:** no other module may update stock directly. Every
  mutation must create an inventory transaction through this domain.

## Reporting (`internal/domain/reporting`)

- **Owns:** tenant-scoped operational read queries, range/filter validation,
  dashboard response composition, and reporting pagination.
- **Does not own:** marketplace/order state, batch or print state, product
  identity, inventory balances, or movement rules.
- **Allowed dependencies:** authenticated principals, centralized
  authorization, and read-only queries over approved authoritative domains.
- **Forbidden leakage:** reporting must not maintain independent counters,
  mutate source records, infer stock from PDFs, or disclose inventory without
  inventory entitlement and permission.

## Returns (`internal/domain/returns`)

- **Owns:** normalized-order cancellation records, expected/received physical
  returns, inspection disposition, lifecycle closure, idempotency, and
  append-only return/cancellation history.
- **Does not own:** marketplace parsing, Product Master identity, inventory
  balances, ledger mutation mechanics, or reporting counters.
- **Allowed dependencies:** normalized marketplace orders, canonical Product
  Master items, authorization, audit, PostgreSQL, and the explicit Inventory
  return-restock boundary.
- **Forbidden leakage:** cancellation, receipt, inspection, and closure must
  never mutate stock; only an authorized restockable disposition may invoke
  Inventory.

## Consignment (`internal/domain/consignment`)

- **Owns:** tenant consignment/SO traceability, configurable departments and
  memberships, canonical product requirements, line progress, workflow state,
  cancellation, idempotency, and append-only consignment history.
- **Does not own:** Product Master identity, inventory balances or ledger
  mechanics, role names, marketplace parsing, or reporting counters.
- **Allowed dependencies:** Product Master IDs, company employees, centralized
  authorization, audit, PostgreSQL, and Inventory's transaction-scoped
  reservation/release/outbound boundary.
- **Forbidden leakage:** consignment code must not update stock tables directly,
  infer departments from employee names, treat pouch references as globally
  unique, or complete partially prepared work.

## Printing (`internal/domain/printing`, `internal/platform/printeragent`, `internal/domain/batch`, and `internal/platform/documents/pdf/generator`)

- **Owns:** print-ready output, artifact traceability, reusable library PDFs,
  registered printers/agents, canonical physical jobs, delivery leases, and
  traceable retries/reprints.
- **Does not own:** source order parsing, Product Master, stock deductions, or
  authorization infrastructure.
- **Allowed dependencies:** normalized labels/orders, authorized actors,
  Product Master references, object storage, approved batch relationships, and
  the local agent's narrow OS-printer backend.
- **Forbidden leakage:** reprinting must never imply inventory movement, and
  marketplace geometry must not be guessed without representative evidence.
  Browser and agent input must never become a command, local path, storage key,
  or unrestricted print option.

## Audit (`internal/platform/audit`)

- **Owns:** consistent persistence of important actor/action/target metadata in
  the caller's transaction.
- **Does not own:** deciding every domain event or implementing domain behavior.
- **Allowed dependencies:** authenticated actor/company identifiers and the
  active PostgreSQL transaction supplied by an owning domain.
- **Forbidden leakage:** domains must not rewrite audit history or record events
  under an unrelated company.

## Object storage (`internal/platform/objectstorage`)

- **Owns:** storage, retrieval, deletion, key containment, and implementation
  details for binary objects. Local filesystem and S3-compatible
  implementations exist behind the same interface.
- **Does not own:** tenant authorization, source-file metadata, parsing,
  duplicate rules, or marketplace job states.
- **Allowed dependencies:** platform configuration and the approved AWS SDK v2
  used for SigV4-compatible object storage.
- **Forbidden leakage:** storage implementations must not make business or
  tenant-access decisions; business services must use the interface rather
  than direct filesystem calls.

## PDF extraction (`internal/platform/documents/pdf/extractor`)

- **Owns:** bounded conversion of an untrusted PDF into numbered page text. The
  current implementation invokes Poppler and offers opt-in Tesseract OCR for
  pages with no extractable text.
- **Does not own:** Flipkart field interpretation, tenant context, Product
  Master resolution, persistence, duplicates, or job state decisions.
- **Allowed dependencies:** bounded document-processing tools or approved
  specialized workers.
- **Forbidden leakage:** extraction tools must not become a second business
  backend or persist authoritative business records.

## PDF generation (`internal/platform/documents/pdf/generator` and marketplace adapters)

- **Owns:** bounded normalized PDF generation contracts, shared Flipkart A4
  rendering, and complete-source-page preservation. Marketplace-specific output
  rules remain in their marketplace adapter; Amazon owns validated A4 enrichment.
- **Does not own:** tenant authorization, batch state, Product Master mapping,
  artifact persistence, reprint policy, or inventory mutations.
- **Forbidden leakage:** generation must not infer missing SKU/quantity, obscure
  required shipping content, or create inventory transactions.

## Automation (Phase 14)

`internal/domain/automation` owns approved printing rules, schedule calculation,
PostgreSQL scheduler/leases, execution history, REST APIs and derived print
reporting. Batch and Consignment persist facts through
`platform/domainevent`; Printing owns queue creation and physical delivery.
Automation has no Inventory dependency. See `workflows/automation.md`.

## Structural consolidation and future ownership

ADR-0006 changes package locations, not responsibility. app retains composition;
Automation retains scheduling; Marketplace retains shared orchestration. Authentication,
authorization, audit, configuration, health and printer-agent mechanics move to platform.

Phase 17 introduced Product ownership around the existing Consignment-era Department
identity without duplicating it. Phase 20's `internal/domain/traceability` owns Trace Boxes,
opaque identifiers, Product content relationships, custody and trace history without Inventory
balances. It reads canonical Product, employee and department references but does not own their
lifecycles. Phase 21 adds full-box QC, generated rework requirements, two-step handovers and
packing/final gates to that owner; the module still does not change Inventory. Seller-account
business state belongs to Phase 16, not to printer-agent/workstation infrastructure.
