# Phase 15 verification record

Baseline: d90e4f7c0ceab032bc33a0618e1ceabdc20906b5, migration 000022.
Verification dates: 2026-09-07 through 2026-09-08.

## Baseline gate

PASS: make verify-full with TEST_DATABASE_URL pointing to a dedicated disposable local
PostgreSQL instance, migrated through 000022. Backend formatting, vet, uncached Go tests,
server/printer-agent builds and frontend typecheck, lint and production build passed.
Dependencies installed with pnpm install --offline --frozen-lockfile; no lockfile change.
The temporary baseline log did not survive the session restart; this record reflects the
observed successful command. The final gate was rerun and its output is retained durably.

This is a fresh local verification result, not a remote CI result. Optional private fixture
inputs were not configured; individual skips are inventoried at the final gate. Physical
printer hardware and interactive browser operation have not been tested by this refactor.

## Structural batches and final gate

PASS. The structural batches were independently verified and committed:

- `30249ad` — platform foundation packages
- `9148c38` — Core, Product, Inventory and Reporting domains
- `a4a228b` — Batch, Printing, Returns, Consignment and Automation domains
- `f38ec27` — Marketplace and all five adapters/fixtures
- `9a55e59` — separate PDF extractor and generator packages
- `8eb1635` — current documentation paths and both CI executable builds

Each batch passed formatting, vet, PostgreSQL-backed uncached Go tests and builds for
`cmd/server` and `cmd/printer-agent`. Relative migration and cross-package fixture paths
were corrected where the added package depth required it. No business behavior changed.

Final `make verify-full` passed on 2026-09-08 against the dedicated disposable database
migrated through 000022. It covered backend formatting, vet, uncached Go tests, both
executables, frontend typecheck, lint, production build and `git diff --check`.

The retained final log is:

`/home/sigma/work/commerceops-exports/phase15-final-verify-20260908.log`

SHA-256: `1b30d0979b9adad6e273fbc2765cc1f57338cc1dc8d871e48937d0ecab933f17`

## Integrity and review

- All migration filenames and bytes through 000022 match the Phase 14 manifest.
- Migration 000023 is absent from this branch and preserved with deferred Phase 16 work.
- `docs/openapi.yaml`, Go dependency files and frontend package/lock files match Phase 14.
- No old Go internal import paths remain; current navigation docs use the new layout.
- The 10 export/recovery regression tests passed, including corruption, traversal,
  dirty-source, existing-destination, exact-ref and historical-secret refusal cases.
- Generated Fontconfig cache files were removed before every commit and the final status check.

## Skipped and not tested

Six optional private-production-fixture tests skipped because their environment variables
were not supplied: two Amazon, two Flipkart (parser and generator), one Meesho and one
Snapdeal. Sanitized fixture tests passed. Physical printer hardware, interactive browser
behavior, real Myntra print capture and remote CI were not tested. No deployment occurred.

Phase 15 implementation is locally verified and awaits owner approval. Phase 16 remains
unauthorized.
