# Phase 21 verification

Date: 2026-09-11  
Branch: `me/phase-21-qc-rework-handover-packing`  
Implementation head before this evidence commit: `8401a8a`

## Result

Phase 21 QC, rework, handover and packing passed its local PostgreSQL-backed completion gate.
The final verification database was a fresh disposable PostgreSQL 17 container migrated in
filename order through `000028_traceability_worker_workflows`.

## Commands executed

- Applied migration `000028`, rolled it down once and applied it again against the launcher-owned
  local database: passed.
- `TEST_DATABASE_URL=<local PostgreSQL> go test ./internal/domain/traceability ./internal/app -count=1`:
  passed.
- `TEST_DATABASE_URL=<local PostgreSQL> go test -race ./internal/domain/traceability -count=1`:
  passed with PostgreSQL integration tests enabled.
- `TEST_DATABASE_URL=<fresh disposable PostgreSQL 17 database> make verify-full`: passed. This
  included Go formatting, `go vet ./...`, uncached Go tests with PostgreSQL enabled, builds of the
  server, printer agent and local launcher, frontend typecheck, frontend lint, frontend production
  build and `git diff --check`.
- Parsed `docs/openapi.yaml` and asserted all seven Phase 21 operation IDs: passed.
- Compared against Phase 20 commit `4bfe6b1`: migration bytes `000001`–`000027`, Go dependency
  manifests, frontend package manifest and frontend lockfile are unchanged.
- Searched backend/frontend source for obsolete pre-Phase-15 package imports: none found.
- Rebuilt and launched `CommerceOps`; frontend returned HTTP 200 and the unauthenticated protected
  API returned 401. Saved login then completed a live content, passing QC, packing, passing final
  check, shipment readiness, employee handover-send and target-receipt flow: passed with eight
  immutable events and final `handover_received` state.

The final full-gate output is retained locally at
`/tmp/commerceops-phase21-final.uU981x/verify-full.log` for this workstation session.

## Regression coverage

Automated PostgreSQL tests cover full-current-content quantity bounds, automatic typed work from
rejected QC, idempotent QC replay, required work completion, mandatory fresh QC before packing,
failed-final-check readiness rejection, successful repacking/final/readiness order, target and
department-member receipt, in-transit mutation rejection, atomic custody on receipt, immutable
worker history, zero Inventory transactions and migration `000027`/`000028` round trip. A
concurrent send test proves that box locking permits only one pending handover. Existing tests
continue to cover company isolation, permissions, module entitlement, duplicate requests and
bounded concurrent content removal.

## Failures resolved during verification

The first PostgreSQL run showed that expanding custody event types had removed the legacy default,
which broke direct custody transfers, and that the Phase 20 migration test needed to remove the
dependent Phase 21 schema before rolling back Phase 20. The migration now preserves the default,
and the round-trip test follows migration order. Both focused and full suites pass afterward.

The first scripted live smoke found that the local database contained no Product. A local-only
smoke Product was added, after which the complete HTTP workflow passed. No source or fixture data
was changed for that setup correction.

## Not tested

Optional private marketplace fixture tests remained skipped because their environment variables
were unset. Physical printer hardware, camera/barcode hardware, generated QR label output, remote
CI and production deployment were not tested. Returns association, Consignment traceability,
pouch/file evidence and mixed-department Consignment progress remain outside Phase 21.
