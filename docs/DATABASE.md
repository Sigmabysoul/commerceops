# Database conventions

PostgreSQL is the sole structured-data store. The Go API connects through `pgx/v5` using the required `DATABASE_URL` environment variable.

Schema changes must be represented by ordered SQL files under `services/api/migrations` and applied explicitly with `golang-migrate`. Application startup does not run migrations.

Phase 1 begins with `000001_core_platform`. It separates global login identities from company access and company-owned employees, roles, entitlements and audit records. Composite foreign keys keep employee access and role assignments within one company. Company and audit history use restrictive deletion behavior; assignment records may cascade only when their owning access or role is removed.

Phase 2 adds `000003_product_master`. Products have company-unique internal codes. SKU mappings use a composite `(company_id, product_id)` foreign key and a partial unique index on active `(company_id, marketplace_key, sku)` mappings. This prevents cross-company product references and makes ambiguous active resolution impossible at the database layer.

Future business tables must carry appropriate company ownership, and business-owned queries must enforce server-established tenant scope. Production schema changes must never be performed manually.

Local PostgreSQL uses the persistent `postgres_data` Docker volume and environment-driven credentials. `.env.example` contains placeholders only.
# Database

PostgreSQL migrations are the schema source of truth. Phase 3 migration `000004_flipkart_processing` adds:

- `source_files` for tenant-owned storage metadata and SHA-256 deduplication
- `processing_jobs` for the persisted state machine and parser version
- `marketplace_orders` and `marketplace_order_items` for normalized results
- `processing_errors` for traceable page warnings and failures

All business foreign keys include company ownership where applicable. Partial unique indexes protect authoritative Flipkart AWB and order identifiers without hiding duplicate review records. Phase 3 creates no inventory table or transaction.
