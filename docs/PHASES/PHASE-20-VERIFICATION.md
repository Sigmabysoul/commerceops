# Phase 20 verification

Date: 2026-09-11  
Branch: `me/phase-20-traceability-foundation`  
Implementation head before this evidence commit: `47c4884`

## Result

Phase 20 Traceability foundation passed its local PostgreSQL-backed completion gate. The
verification database was a fresh disposable native PostgreSQL cluster migrated in filename
order through `000027_traceability_foundation`.

## Commands executed

- Applied migrations `000001`–`000027`, then applied the Phase 20 down migration and up
  migration again: passed.
- `TEST_DATABASE_URL=<disposable database> go test ./internal/domain/traceability ./internal/app ./cmd/local-launcher -count=1`: passed.
- `TEST_DATABASE_URL=<disposable database> go test -race ./internal/domain/traceability -count=1`: passed.
- `TEST_DATABASE_URL=<disposable database> make verify-full`: passed after the final route fix.
  This included Go formatting checks, `go vet ./...`, uncached Go tests with PostgreSQL enabled,
  server/printer-agent/local-launcher builds, frontend typecheck, frontend lint, frontend
  production build and `git diff --check`.
- Parsed `docs/openapi.yaml` and asserted the Phase 20 paths: passed.
- Rebuilt and launched `CommerceOps`; saved administrator login, `/api/v1/trace-boxes`,
  `/api/v1/trace-box-options` and the frontend returned HTTP 200.

The complete final gate output is retained locally at
`/tmp/commerceops-phase20-final.lxtigD/verify-full-route-fix.log` for this workstation session.

## Regression coverage

Automated PostgreSQL tests cover random opaque identifier shape, authenticated tenant-scoped
resolution, cross-company Product/custody rejection, permission and entitlement denial,
idempotent replay and conflicting replay, serialized concurrent quantity removal, derived
contents, employee and department custody, unified audit history, all three immutable-history
triggers, zero Inventory transactions, migration down/up and HTTP method behavior. Application
composition tests register every Traceability route and reject ambiguous patterns.

## Failures resolved

The first live smoke exposed ambiguous dynamic `net/http` patterns between identifier resolution
and content operations. Identifier resolution moved to `/api/v1/trace-box-resolutions/{opaque_identifier}`
and a route-registration regression was added. Final full verification and live startup passed
after that correction.

## Not tested

Optional private marketplace fixture tests remained skipped because their environment variables
were unset. Physical printer hardware, camera-based barcode scanning, generated QR label output,
remote CI and production deployment were not tested. QR label printing, QC/rework, handover,
packing, Returns integration and Consignment integration are outside Phase 20.
