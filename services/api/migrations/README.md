# Database migrations

Schema changes are applied explicitly with `golang-migrate`; application startup never runs migrations.

Migration files use the sequential format `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`.

`000001_core_platform` establishes the Phase 1 company, identity, employee, authorization, entitlement, session and audit tables. `000002_tenant_sessions` binds each session to a verified company membership with a composite foreign key. `000003_product_master` adds tenant products, normalized marketplace references and deterministic SKU mappings. `000004_flipkart_processing` adds Phase 3 source, job, order, item, and error records. `000005_flipkart_worker_leases` adds multi-instance-safe processing ownership and lease expiry. `000006_batch_foundation` adds Phase 4 tenant batches, ordered membership, idempotency, and batch/printing permission definitions. `000007_print_generation` adds traceable print jobs, ordered source items, and storage-backed artifact metadata. `000008_worker_assignments_reprints` adds configurable exact-product/fallback worker rules, ready-batch assignment snapshots, and source-linked reprint metadata. Apply pending migrations through the repository command after PostgreSQL is healthy:

`000009_inventory_ledger` adds the immutable Phase 5 inventory ledger, locked
balance cache, and granular inventory permissions.

`000010_inventory_outbound_reservations` adds unique ready-batch outbound events,
the `ecommerce_out` ledger type, source-linked reservation lifecycle, supporting
indexes, and `inventory.dispatch` permission.

`000011_dashboard_reporting` adds the basic reporting permission and
company/date/status indexes. It adds no reporting tables or duplicated counters.

`000012_amazon_processing` adds the partial durable-claim index for Amazon
queued/processing jobs. Amazon reuses the existing normalized marketplace tables.

`000013_amazon_document_association` adds tenant-scoped contributing-document
traceability for Amazon label/invoice associations without duplicating orders.

`000014_returns_cancellations_foundation` adds tenant-scoped cancellation
records, return cases/items, append-only return intake events, and granular
returns permissions. It does not add a return-owned stock counter or mutate the
inventory ledger.

```sh
make migrate
```

Docker is optional. With native PostgreSQL and `golang-migrate` installed, use the same migration files directly:

```sh
migrate -path services/api/migrations -database "$DATABASE_URL" up
```

PostgreSQL-backed tests require an already migrated, disposable database:

```sh
TEST_DATABASE_URL="$DATABASE_URL" go test ./... -v
```
