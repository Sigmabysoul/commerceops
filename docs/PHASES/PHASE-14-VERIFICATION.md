# Phase 14 implementation verification

Date: 2026-09-05. Branch: `phase/14-printing-automation`.
Base HEAD: `7bc1dc43239b110fcc2f2506841a70f3e503e279`.
The implementation is in the working tree; no new commit or remote CI run was
created as part of this task.

## Delivered scope

- Migration `000022`: timezone, rules, durable events, executions, dedicated
  permissions, tenant-composite foreign keys, unique occurrences, claim indexes,
  and the automation physical-job origin.
- Daily/weekday/selected-day schedules, multiple clock times, inclusive dates,
  explicit company timezone snapshots, and preview/execution calculation parity.
- Transactional Batch-ready and Consignment-packing/packed event hooks.
- Bounded PostgreSQL scheduler, startup/shutdown, leases, stale-worker fencing,
  restart catch-up, atomic execution/job completion and semantic idempotency.
- Copy bounds, derived daily limit, offline/inactive failure state, exponential
  backoff, audited auto-pause, optimistic edits, explicit tests and safe retries.
- Typed APIs, responsive workspace, upcoming/runs/failures/history, and derived
  automatic/manual copy counts and physical printer outcomes.
- All jobs use the normal Printing queue and agent delivery path. No inventory
  writes, hardware shortcuts, new dependencies, brokers, or later-phase domains.

## Commands and results

1. Applied migrations `000001` through `000022`, each transactionally with
   `psql -v ON_ERROR_STOP=1 -1 -f`, to a newly created disposable local database.
   Passed. Automation tests also create independent migrated schemas.
2. `go test ./internal/automation ./internal/batch ./internal/consignment
   ./internal/printing -count=1` with `TEST_DATABASE_URL` set: passed.
3. Final `TEST_DATABASE_URL=... make verify-full`: passed, exit 0. This executed
   `gofmt -l`, `go vet ./...`, `go test ./... -count=1`, server and printer-agent
   builds, `pnpm typecheck`, `pnpm lint`, `pnpm build`, and `git diff --check`.
   PostgreSQL tests were explicitly enabled, not skipped.
4. Separate OpenAPI validation using PyYAML: passed parsing, duplicate-key and
   operation-ID checks, and resolution of all local references.
5. Optional private fixture audit:
   `go test ./internal/marketplace/amazon ./internal/marketplace/flipkart
   ./internal/marketplace/meesho ./internal/marketplace/snapdeal
   ./internal/platform/pdfgenerator -run 'TestPrivate|TestRepresentativePrivate'
   -v -count=1`. All six selected tests were **skipped**, not verified.

The final full-gate log for this workspace session is
`/tmp/commerceops-phase14-verified.log`.

## Regression evidence

`internal/automation` covers Indian timezone/day boundaries; weekday and
selected-day schedules; inclusive start/end dates; New York missing/repeated
DST minutes; Lord Howe's half-hour transition; duplicate times/invalid zones;
concurrent materialization; restart after materialization and expired lease;
concurrent test-run replay; atomic rollback after Printing insertion; company
edit locks allowing worker foreign-key checks; disabled rules; inactive assets;
offline printer/backoff/auto-pause; manual execution retry; concurrent daily
copy limits; tenant and permission isolation; optimistic conflicts; immutable
rule timezone until explicit edit; preview parity; event duplicate/rollback/
historical-rule matching; normal agent PDF delivery/completion; visible physical
failure without automatic reprint; derived manual/automatic metrics; audit;
Inventory neutrality; scheduler cancellation; and migration down/up plus the
historical physical-job downgrade guard.

Batch and Consignment regression tests invoke the owning domain's actual
transition methods and verify event type/count, replay safety, rejected
transition behavior, and unchanged stock/reservation semantics.

## Unverified and operational limits

- Physical printer/CUPS hardware and interactive desktop/mobile browser flows
  were not exercised; agent protocol and backend/frontend automated checks ran.
- `AMAZON_PRIVATE_SAMPLE`, `FLIPKART_PRIVATE_SAMPLES_DIR`,
  `MEESHO_PRIVATE_SAMPLE`, and `SNAPDEAL_PRIVATE_SAMPLE` were absent. Their six
  optional production-PDF checks were skipped; sanitized regressions passed.
- Remote CI is not asserted green for this uncommitted implementation.
- Deployment must apply `000022` before starting the server and grant the new
  automation permissions to the intended roles. No production migration ran.
- Event strategies deliberately print configured Print Library assets, as
  approved; they do not guess marketplace-generation artifacts.
- Physical delivery ambiguity still requires the Printing domain's explicit
  operator-reviewed retry. Automation retries never recreate an existing job.

Phase 14 is implemented and locally verified. Further phases require explicit
owner authorization.
