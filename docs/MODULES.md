# Module Ownership

CommerceOps is a modular monolith. Ownership describes responsibility boundaries
inside one application; it does not imply microservices.

## Authentication (`internal/auth`)

- **Owns:** login identity verification, password hashing, session creation,
  authentication cookies, logout, and authenticated principal construction.
- **Does not own:** roles, permissions, company business data, or marketplace
  processing.
- **Allowed dependencies:** PostgreSQL platform access and shared HTTP response
  utilities.
- **Forbidden leakage:** handlers or other domains must not construct trusted
  tenant principals from request-provided `company_id` values.

## Authorization (`internal/authorization`)

- **Owns:** centralized permission and module-entitlement decisions for an
  authenticated principal.
- **Does not own:** authentication, role administration UI, or business-domain
  rules.
- **Allowed dependencies:** authenticated principals and tenant-scoped role,
  permission, and entitlement records.
- **Forbidden leakage:** domains and frontend components must not replace
  authorization checks with role-name checks or UI visibility.

## Core and company (`internal/core`)

- **Owns:** companies, employees, company user access, roles, permission
  assignments, module-entitlement administration, and audit-log retrieval.
- **Does not own:** authentication internals, canonical products, marketplace
  parsing, inventory, or printing.
- **Allowed dependencies:** authentication principals, centralized
  authorization, audit recording, and PostgreSQL.
- **Forbidden leakage:** company ownership must not be inferred from frontend
  input, and employee assignments must not be hardcoded in other modules.

## Product Master (`internal/product`)

- **Owns:** canonical tenant products, lifecycle state, marketplace SKU
  mappings, deterministic exact resolution, and Product Master training.
- **Does not own:** marketplace document parsing, order ingestion, inventory
  balances, or worker assignment.
- **Allowed dependencies:** authenticated principals, authorization, audit, and
  normalized marketplace keys.
- **Forbidden leakage:** marketplace SKU strings must never become canonical
  product identity, and marketplace processors must not invent products.

## Marketplace (`internal/marketplace`)

- **Owns:** marketplace upload orchestration, source/job metadata, normalized
  marketplace orders/items, duplicate detection, processing states, review
  errors, database-backed job leases, and marketplace-specific adapters such
  as Flipkart.
- **Does not own:** canonical product definitions, authentication,
  authorization policy, object persistence mechanics, inventory, batches, or
  printing.
- **Allowed dependencies:** Product Master resolution, authorization, audit,
  PostgreSQL, object-storage interfaces, and PDF-extraction interfaces.
- **Forbidden leakage:** Flipkart and Amazon parsing rules must remain in their
  isolated adapters; marketplace processing must not mutate inventory or
  implement future marketplace adapters outside the active phase.

## Inventory (`internal/inventory`)

- **Owns:** stock ledger transactions, balances, source-linked reservations,
  ready-batch ecommerce outbound confirmation, adjustments, corrections, and
  inventory idempotency.
- **Does not own:** marketplace parsing, returns disposition, printing, or
  product identity.
- **Allowed dependencies:** Product Master identities, authenticated actors,
  authorization, audit, and approved domain references.
- **Forbidden leakage:** no other module may update stock directly. Every
  mutation must create an inventory transaction through this domain.

## Reporting (`internal/reporting`)

- **Owns:** tenant-scoped operational read queries, range/filter validation,
  dashboard response composition, and reporting pagination.
- **Does not own:** marketplace/order state, batch or print state, product
  identity, inventory balances, or movement rules.
- **Allowed dependencies:** authenticated principals, centralized
  authorization, and read-only queries over approved authoritative domains.
- **Forbidden leakage:** reporting must not maintain independent counters,
  mutate source records, infer stock from PDFs, or disclose inventory without
  inventory entitlement and permission.

## Returns (`internal/returns`)

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

## Consignment (`internal/consignment`)

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

## Printing (future, locked)

- **Owns:** when approved, print-ready output, print jobs, printer state, and
  traceable reprints.
- **Does not own:** source order parsing, Product Master, stock deductions, or
  authorization infrastructure.
- **Allowed dependencies:** normalized labels/orders, authorized actors, object
  storage, and approved batch relationships.
- **Forbidden leakage:** reprinting must never imply inventory movement. No
  printing implementation is authorized during Phase 3.

## Audit (`internal/audit`)

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

## PDF extraction (`internal/platform/pdfextractor`)

- **Owns:** bounded conversion of an untrusted PDF into numbered page text. The
  current implementation invokes Poppler and offers opt-in Tesseract OCR for
  pages with no extractable text.
- **Does not own:** Flipkart field interpretation, tenant context, Product
  Master resolution, persistence, duplicates, or job state decisions.
- **Allowed dependencies:** bounded document-processing tools or approved
  specialized workers.
- **Forbidden leakage:** extraction tools must not become a second business
  backend or persist authoritative business records.

## PDF generation (`internal/platform/pdfgenerator` and marketplace adapters)

- **Owns:** bounded normalized PDF generation contracts and shared Flipkart A4
  rendering. Marketplace-specific output rules remain in their marketplace
  adapter; Amazon owns validated A4 enrichment.
- **Does not own:** tenant authorization, batch state, Product Master mapping,
  artifact persistence, reprint policy, or inventory mutations.
- **Forbidden leakage:** generation must not infer missing SKU/quantity, obscure
  required shipping content, or create inventory transactions.
