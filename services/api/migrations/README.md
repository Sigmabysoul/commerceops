# Database migrations

Schema changes are applied explicitly with `golang-migrate`; application startup never runs migrations.

Migration files use the sequential format `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`.

`000001_core_platform` establishes the Phase 1 company, identity, employee, authorization, entitlement, session and audit tables. Apply it with the Compose migration profile after PostgreSQL is healthy:

```sh
docker compose --profile tools run --rm migrate
```
