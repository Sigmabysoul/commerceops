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
