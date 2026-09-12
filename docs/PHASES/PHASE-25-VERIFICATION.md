# Phase 25 verification

Date: 2026-09-12

Branch: `me/phase-25-production-hardening`

Runtime, packaging and rollout-guide head: `1b44aa6`

Final PostgreSQL-backed verification head: `436f139`

## Result

The Phase 25 repository implementation passed its local production-readiness gate. CommerceOps
now has strict production configuration, hardened HTTP boundaries, non-root API and web images,
external PostgreSQL/S3 production composition, backup-bound release evidence, concurrent smoke
measurement, graceful-shutdown behavior and same-schema application rollback planning.

Production rollout and operator acceptance remain pending. Local evidence does not establish the
real TLS proxy, S3 provider, database service, central logging and alerts, scheduled backups,
recovery objectives or operator workflow. The repository does not authorize or perform an
automatic deployment.

## Full verification

An already migrated disposable PostgreSQL 17 database at migration version 30 was started for the
final gate. `TEST_DATABASE_URL=<disposable PostgreSQL> make verify-full` passed. It included:

- Go formatting and `go vet ./...`;
- uncached Go tests with PostgreSQL integration enabled across application, company isolation,
  authorization, Marketplace, Automation, Inventory, Printing, Returns, Consignment,
  Traceability and Reporting packages;
- builds of `cmd/server`, `cmd/printer-agent` and `cmd/local-launcher`;
- frontend typecheck, zero-warning lint and Next.js 15.5.24 production build;
- all four lifecycle backup/restore/archive tests;
- all five production-readiness configuration, smoke, release and rollback tests; and
- `git diff --check`.

The final output is retained locally at
`/tmp/commerceops-phase25-final.ewWHPU/verify-full.log`. Its SHA-256 is
`f8f0ac25a5f67ed2b164b06e82d38d04adb22c5f3267bbbea02a7f4fd756ce2c`.

## Image and runtime rehearsal

Both production Dockerfiles built successfully. The locally verified image IDs are:

- API: `sha256:05c69bfdfc6157c669f058e0fae1f9c7342df9ae8800a39abc8d14957c98c400`
- web: `sha256:3336ba9ac8d00af3af5850b5ed8009d537255359c7a64aebc95603e69f0beb4a`

The production-like rehearsal ran the images as non-root users with read-only root filesystems,
temporary writable paths, all Linux capabilities dropped and `no-new-privileges`. PostgreSQL was
external to the application composition and migrations completed before API readiness. The web
root returned HTTP 200.

A concurrent readiness run completed 500 requests at concurrency 20 with zero failures. Measured
latency was 80.423 ms median, 182.496 ms p95 and 346.551 ms maximum on this workstation. An
untrusted browser Origin was rejected with HTTP 403 and `ORIGIN_NOT_ALLOWED` before a business
handler. Responses included the configured security and request-correlation headers.

The API received `SIGTERM`, logged shutdown, exited zero in approximately 1.22 seconds and became
ready after restart. A same-migration rollback rehearsal started the retained application images
and completed 25 requests at concurrency 5 with zero failures; latency was 16.694 ms median,
31.470 ms p95 and 35.718 ms maximum. The rollback planner rejects releases whose migration
versions differ and never enables a down migration.

## Recovery and release evidence

The release command was exercised against the verified Phase 24 backup/restore evidence. It bound
the selected commit, migration version 30 and immutable-format API/web digests to the matching
backup receipt. The generated result retained `operator_acceptance: pending` and
`deployment_authorized: false`. Corrupt receipts, mutable image tags, release output inside the
protected backup and schema-mismatched rollback were rejected.

Phase 24 separately proved a full-schema restore of 67 public tables plus byte-for-byte object
restoration. See `PHASE-24-VERIFICATION.md`. Phase 25 does not reinterpret that local drill as a
production recovery-time or recovery-point guarantee.

## Contract impact

Compared with Phase 24 commit `86d41e21b750da801743ca0720aba56f52219ae0`, all migration files,
`docs/openapi.yaml`, Go dependency files, the frontend package manifest and frontend lockfile are
unchanged. Phase 25 adds no schema migration, REST/OpenAPI/type change or business behavior. The
frontend workspace build policy allows the existing native build dependencies required by the
standalone production image; it adds no application dependency.

## Not tested

- Real production domains, reverse-proxy TLS policy and public network routing.
- A live S3-compatible bucket, credentials, retention/versioning policy or provider outage.
- Managed PostgreSQL configuration, failover, connection limits or production data volume.
- Central log retention, alert delivery, escalation handling and dashboards.
- Scheduled production backups, a remote recovery host, or measured production RPO/RTO.
- Remote GitHub Actions execution and private image-registry push/pull.
- Operator acceptance accounts, real internal rollout and post-rollout observation.
- Optional private marketplace fixtures and physical printer, scanner, camera or barcode hardware.

These items are the remaining production rollout gate. No production deployment was attempted.
