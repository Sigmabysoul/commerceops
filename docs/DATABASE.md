# Database conventions

PostgreSQL is the sole structured-data store. The Go API connects through `pgx/v5` using the required `DATABASE_URL` environment variable.

Schema changes must be represented by ordered SQL files under `services/api/migrations` and applied explicitly with `golang-migrate`. Application startup does not run migrations. Phase 0 has no schema requirement and therefore no ceremonial migration.

Future business tables must carry appropriate company ownership, and business-owned queries must enforce server-established tenant scope. Production schema changes must never be performed manually.

Local PostgreSQL uses the persistent `postgres_data` Docker volume and environment-driven credentials. `.env.example` contains placeholders only.
