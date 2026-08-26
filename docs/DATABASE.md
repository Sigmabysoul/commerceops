# Database conventions

PostgreSQL is the sole structured-data store. The Go API connects through `pgx/v5` using the required `DATABASE_URL` environment variable.

Schema changes must be represented by ordered SQL files under `services/api/migrations` and applied explicitly with `golang-migrate`. Application startup does not run migrations.

Phase 1 begins with `000001_core_platform`. It separates global login identities from company access and company-owned employees, roles, entitlements and audit records. Composite foreign keys keep employee access and role assignments within one company. Company and audit history use restrictive deletion behavior; assignment records may cascade only when their owning access or role is removed.

Future business tables must carry appropriate company ownership, and business-owned queries must enforce server-established tenant scope. Production schema changes must never be performed manually.

Local PostgreSQL uses the persistent `postgres_data` Docker volume and environment-driven credentials. `.env.example` contains placeholders only.
