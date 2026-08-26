# Database migrations

Schema changes are applied explicitly with `golang-migrate`; application startup never runs migrations.

Migration files use the sequential format `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`. Phase 0 has no schema requirement, so it intentionally contains no SQL migration.
