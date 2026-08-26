# Database migrations

Schema changes are applied explicitly with `golang-migrate`; application startup never runs migrations.

Migration files use the sequential format `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`.

`000001_core_platform` establishes the Phase 1 company, identity, employee, authorization, entitlement, session and audit tables. `000002_tenant_sessions` binds each session to a verified company membership with a composite foreign key. `000003_product_master` adds tenant products, normalized marketplace references and deterministic SKU mappings. Apply them with the Compose migration profile after PostgreSQL is healthy:

```sh
docker compose --profile tools run --rm migrate
```

Docker is optional. With native PostgreSQL and `golang-migrate` installed, use the same migration files directly:

```sh
migrate -path services/api/migrations -database "$DATABASE_URL" up
```

PostgreSQL-backed tests require an already migrated, disposable database:

```sh
TEST_DATABASE_URL="$DATABASE_URL" go test ./... -v
```
