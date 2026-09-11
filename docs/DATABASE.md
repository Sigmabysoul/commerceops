# Database conventions

PostgreSQL is the sole structured-data store. The Go API connects through `pgx/v5` using the required `DATABASE_URL` environment variable.

Schema changes must be represented by ordered SQL files under `services/api/migrations` and applied explicitly with `golang-migrate`. Application startup does not run migrations.

Phase 1 begins with `000001_core_platform`. It separates global login identities from company access and company-owned employees, roles, entitlements and audit records. Composite foreign keys keep employee access and role assignments within one company. Company and audit history use restrictive deletion behavior; assignment records may cascade only when their owning access or role is removed.

Phase 2 adds `000003_product_master`. Products have company-unique internal codes. SKU mappings use a composite `(company_id, product_id)` foreign key and a partial unique index on active `(company_id, marketplace_key, sku)` mappings. This prevents cross-company product references and makes ambiguous active resolution impossible at the database layer.

Phase 3 migration `000005_flipkart_worker_leases` adds paired `worker_id` and
`lease_expires_at` fields to `processing_jobs`. Flipkart workers may claim only
queued jobs or processing jobs whose lease has expired. The partial claim index
is marketplace- and status-scoped; tenant foreign keys and normalized order
constraints remain unchanged.

Future business tables must carry appropriate company ownership, and business-owned queries must enforce server-established tenant scope. Production schema changes must never be performed manually.

Local PostgreSQL uses the persistent `postgres_data` Docker volume and environment-driven credentials. `.env.example` contains placeholders only.
# Database

PostgreSQL migrations are the schema source of truth. Phase 3 migration `000004_flipkart_processing` adds:

- `source_files` for tenant-owned storage metadata and SHA-256 deduplication
- `processing_jobs` for the persisted state machine and parser version
- `marketplace_orders` and `marketplace_order_items` for normalized results
- `processing_errors` for traceable page warnings and failures

All business foreign keys include company ownership where applicable. Partial unique indexes protect authoritative Flipkart AWB and order identifiers without hiding duplicate review records. Phase 3 creates no inventory table or transaction.

Phase 4 Batch A migration `000006_batch_foundation` adds tenant-owned `batches`
and ordered `batch_members`. Composite foreign keys keep creators, batches, and
normalized marketplace orders in the same company. Company/order uniqueness
prevents silent inclusion in multiple operational batches, while a
company/idempotency-key constraint makes creation safely replayable. Counts and
Product Master totals are derived; no inventory or print-artifact tables are
introduced in Batch A.

Phase 4 Batch B migration `000007_print_generation` adds `print_jobs`, ordered
`print_job_items`, and immutable `print_artifacts`. Tenant-composite foreign keys
preserve the batch, normalized order, source file, processing job, and source
page relationship. Artifacts record storage keys, hashes, sizes, page counts,
and generation configuration. These records have no inventory side effects.

Phase 5 migration `000009_inventory_ledger` adds tenant/Product Master scoped
`inventory_balances` and immutable `inventory_transactions`. Balance rows are a
transactionally locked cache; ledger entries preserve previous/resulting
balances, actor, reason, reference, request hash, and idempotency key. A
database trigger rejects ledger updates and deletes.

Migration `000010_inventory_outbound_reservations` adds unique ready-batch
outbound events and source-linked reservations. One ecommerce ledger entry is
allowed per company/batch/product. Reservation source uniqueness prevents
duplicate holds; company/status/product indexes support bounded operational
reads. Reservation create/release updates cached `reserved` atomically without
changing on-hand stock.

Phase 6 migration `000011_dashboard_reporting` adds `reports.view` and
company/date/status indexes supporting reporting filters. It introduces no
report counters or aggregate tables: marketplace, batch, print, and inventory
tables remain authoritative and reporting queries are rebuild-free by design.

Phase 7 Batch A migration `000012_amazon_processing` adds only an Amazon-scoped
partial claim index over the existing `processing_jobs` table. Amazon source
files, jobs, normalized orders/items, errors, leases, and duplicate constraints
reuse the generic marketplace schema; no Amazon-only business tables exist.

Phase 7 Batch B migration `000013_amazon_document_association` adds the generic
tenant-scoped `marketplace_order_documents` child relation. It preserves every
contributing source page, role, source file, and extraction method for normalized
orders and later shared print-artifact traceability.

Phase 8 migrations `000014` through `000016` add tenant-scoped cancellation and
physical-return cases, bounded Product Master return items, append-only lifecycle
events, centralized `return_restock` ledger support, compensating correction
traceability, and explicit closure actors/timestamps. Returns never maintain a
second stock balance; Inventory balances and the immutable ledger remain the
only stock authority.

Phase 9 migration `000017_consignment_management` adds configurable
tenant-owned departments and employee memberships, consignments, canonical
Product Master lines, optimistic line versions, and immutable events. Composite
foreign keys enforce company ownership. Order/SO references are unique inside a
company; pouch/file references are indexed but deliberately not unique because
the operating scope does not establish global uniqueness. The migration also
adds `consignment_out` to the immutable Inventory ledger and permits one such
entry per company/consignment/product. Reservations continue to use the Phase 5
table: cancellation and outbound both close the active reservation, while the
recorded release reason distinguishes release from outbound consumption.

Phase 10 migration `000018_meesho_processing` adds only a Meesho-scoped
partial claim index over existing `processing_jobs`. Meesho source files, jobs,
normalized orders/items, source-page documents, errors, leases, Product Master
mappings, and duplicate constraints reuse the generic marketplace schema. No
Meesho-only business table or inventory schema change is introduced.

Phase 10's remaining batch, print, outbound, reporting, and returns integration
requires no additional migration: those domains already reference normalized
marketplace keys and tenant-owned generic relations.

## Phase 14 automation

Migration `000022_printing_automation` adds `companies.timezone`,
`automation_rules`, `automation_domain_events`, and `automation_executions`,
plus dedicated permissions and the `automation` physical job origin. Composite
tenant foreign keys protect rule/asset/printer/event/job relationships. Unique
rule occurrence keys and job identities prevent duplicate queue creation. No
Inventory or reporting counter schema changes. Down migration refuses to erase
origins of existing automation jobs. See `workflows/automation.md`.

## Phase 15 migration freeze

The selected baseline is d90e4f7c0ceab032bc33a0618e1ceabdc20906b5, through migration 000022.
All migration names and bytes are frozen for Phase 15. Compare CODEX/BASELINE_MIGRATIONS.sha256
before and after consolidation. The deferred seller-account migration 000023 is preserved
on a separate branch and is not part of this baseline. Do not apply it to Phase 15 tests.

The single-business product direction preserves company-scoped constraints and existing
entitlements. Later schema changes require new migrations in their approved phase. Use
only a dedicated disposable migrated database for verify-full; startup never migrates.

## Phase 16 seller-account foundation

Migration `000023_marketplace_seller_accounts` introduces company-scoped
`business_identities` and `marketplace_accounts`, with permissions for viewing and managing
them. Accounts are bound to an existing marketplace key and a business identity through a
composite company foreign key. They are configuration records only in this initial migration:
they neither identify a workstation nor change inventory, printing, or marketplace parsing.

Migration `000024_marketplace_account_provenance` records an explicit account ID on SKU
mappings, source files, processing jobs, and normalized marketplace orders. Existing data is
backfilled to dormant unassigned-legacy accounts; this preserves history without guessing a
seller. New account-aware uniqueness rules scope SKU, source-file, idempotency, AWB, and order
deduplication by seller account, while partial legacy indexes retain historical duplicate safety.

Migration `000025_marketplace_account_workflow_provenance` carries that immutable account
context into batches, batch members, cancellations, return cases, and marketplace-order
documents. Child records inherit the account only from their normalized marketplace order;
batches reject a mixed-account order selection. Composite foreign keys keep each workflow
record aligned with its parent account without changing inventory ownership or movement rules.

## Phase 17 Product department ownership

Migration `000026_product_department_ownership` reuses `consignment_departments` and adds
effective-dated `product_department_assignments`. A partial unique index permits at most one
active assignment per company and Product. Reassignment closes the previous interval while
existing `consignment_lines.department_id` values remain immutable snapshots of their routing.

## Phase 20 Traceability foundation

Migration `000027_traceability_foundation` adds company-scoped `trace_boxes` with random opaque
identifiers, unified immutable `trace_box_events`, and event-linked content and custody changes.
Content changes reference canonical Products with signed explicit quantities. Custody changes
reference exactly one same-company employee or department. Current contents and custody are
derived from this history. Database triggers reject event updates and deletes. No Inventory
table, balance, reservation or ledger behavior changes.

## Phase 21 Traceability worker workflows

Migration `000028_traceability_worker_workflows` extends the Trace Box event vocabulary and adds
typed immutable records for full-box QC lines, QC-generated work requirements and completions,
two-step handovers and receipts, and packing/final/readiness gates. Rejected QC quantities must
name both a rejection reason and required work. A unique work completion and handover receipt
prevents duplicate mobile retries. Receipt also appends the existing custody history in the same
transaction. Box row locks serialize transition checks. Current workflow state is derived; no
Inventory table or balance changes.

## Phase 22 Consignment traceability integration

Migration `000029_consignment_traceability_integration` adds an opt-in Consignment traceability
flag, immutable signed Trace Box allocations tied to canonical line Product snapshots, and
append-only pouch/file evidence. Active allocations are derived from link and unlink events.
Reference indexes are non-unique by design. No Inventory schema or ledger rule changes.

## Phase 23 operations analytics

Migration `000030_operations_analytics_indexes` adds
`trace_box_events_company_time_idx(company_id,created_at,event_type,id)` for company/time
reporting cohorts. It adds no tables, counters, permissions or data mutations. Reporting reads
typed immutable QC, work, handover and gate facts in a read-only repeatable-read transaction.
Earlier migrations remain unchanged; rolling down `000030` removes only this index.

## Phase 24 data lifecycle

Phase 24 adds no migration or database object. `scripts/lifecycle/lifecycle.py` uses a
serializable custom-format `pg_dump`, records every public table row count and the complete
`schema_migrations` ledger, and verifies those values after restoring into an empty disposable
database. It also compares database object references with the restored byte inventory before
writing a receipt. The procedure is documented in `operations/data-lifecycle.md`.
