# Phase 15 verification checklist

- Preserve source/index state, WIP ref and test-checkout patch; record exact baseline.
- Validate bundle, source ZIP, selected commit and checksum manifests.
- Fresh disposable PostgreSQL migrated through 000022; baseline make verify-full.
- Each batch: formatting, Go tests with PostgreSQL, vet/build and direct importer review.
- Final make verify-full: backend format/vet/uncached tests/server+agent builds and frontend typecheck/lint/build.
- Compare baseline migration filenames/bytes and OpenAPI/dependency hashes.
- Review fixture and migration relative paths, old imports, links and git diff --check.
- Test export integrity rejection, dirty-source policy, existing-destination refusal and exact-ref restoration.
- Record optional private fixture skips, physical hardware/browser limits and remote CI status explicitly.
- Clean committed tree and scope report; no Phase 16 or deployment.

## Phase 24 lifecycle gate

- Run lifecycle unit tests and the real PostgreSQL/object restore drill with `TEST_DATABASE_URL`.
- Validate corruption, traversal, symlink, unfinished upload and existing-destination refusals.
- Back up a database migrated through the current migration, restore into an empty database and
  compare the full database catalog and object manifest.
- Produce a receipt-gated archive dry run; confirm deletion is disabled and protected histories
  are recorded.
- Run PostgreSQL-backed `make verify-full`; record unavailable S3, production scheduling and
  remote recovery evidence explicitly.

## Phase 25 production-readiness gate

- Verify production configuration refuses placeholders, insecure endpoints, local object storage
  and mutable image references.
- Build both production images from a clean checkout; inspect non-root users and run them with
  read-only filesystems, no added capabilities and bounded temporary storage.
- Apply all migrations to disposable PostgreSQL, run concurrent health load and verify zero
  failures plus required request/security headers and untrusted-origin rejection.
- Send `SIGTERM`; record shutdown duration, zero exit status, shutdown log and successful restart.
- Bind a release manifest to Phase 24 backup/restore evidence and rehearse same-schema application
  rollback. Confirm a migration-version mismatch is refused.
- Run PostgreSQL-backed `make verify-full`, container builds and source-export integrity tests.
- Record remote CI, real TLS/proxy, S3, centralized logs/alerts, scheduled backups, production
  recovery timing and operator acceptance as untested until real evidence exists.
