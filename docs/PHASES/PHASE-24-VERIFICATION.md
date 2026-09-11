# Phase 24 verification

Date: 2026-09-11  
Branch: `me/phase-24-data-lifecycle-archival`  
Implementation and operating-guide head before this evidence commit: `9365059`

## Result

Phase 24 data lifecycle and archival passed its local PostgreSQL-backed completion gate. The
toolkit creates and validates a protected PostgreSQL/object snapshot, restores only into empty
destinations, proves database and object equality before writing a receipt, and produces a
receipt-gated archive dry run. There is no deletion command.

## Commands executed

- Migrated a fresh disposable PostgreSQL 17 database through
  `000030_operations_analytics_indexes`: passed with no dirty migration.
- `TEST_DATABASE_URL=<disposable PostgreSQL> make verify-full`: passed. This included Go
  formatting, `go vet ./...`, uncached Go tests with PostgreSQL enabled, builds of the server,
  printer agent and local launcher, frontend typecheck, frontend lint, frontend production build,
  the lifecycle unit/integration suite and `git diff --check`.
- Created a second quiescent database, applied all migrations, ran `make lifecycle-backup` and
  `make lifecycle-validate`, restored into a third empty database and absent object directory,
  then ran `make lifecycle-archive-plan`: passed. The restored catalog matched 67 public tables
  and migration version 30. The receipt records matching table counts and
  `deletion_authorized: false`; the plan records `mode: dry_run` and
  `deletion_enabled: false`.
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/export/test_export.py -v`: all 10 source
  export integrity and restoration tests passed.
- Compared Phase 24 with Phase 23 commit `cceac8f`: migrations, OpenAPI, Go dependency files,
  frontend dependency manifest and lockfile are unchanged. The diff contains only the lifecycle
  toolkit, Make targets and documentation.

The final full-gate output is retained locally at
`/tmp/commerceops-phase24-final.hmm8nS/verify-full.log` for this workstation session. Its SHA-256
is `41dca580b61fb5f130b1f738e0d83c652161f66857ed6ed7b6acb2b32a06a17c`.
The full-schema drill log is in the same directory; its SHA-256 is
`fd25bbb2fb8ca776b68285deb9f4e311c40c19c8a9a447e742e10b7eb4963a32`.

## Lifecycle regression coverage

The manifest suite rejects changed bytes, path traversal, unexpected files, symlinks and
unfinished `.upload` sources. It refuses an existing backup destination, a database reference
whose object is missing, a nonempty restore database and a mismatched restore receipt. Restore
checks the migration ledger, all public table counts, database object references and copied object
bytes. Archive output requires a successful receipt for the same backup, refuses overwrite,
retains audit/Inventory/Traceability history evidence and can never enable deletion.

The PostgreSQL integration fixture restored one real object byte-for-byte and preserved rows from
`audit_logs`, `inventory_transactions` and `trace_box_events`. The separate full-schema drill used
an empty stable object root; together these cover object recovery and the current complete schema.

## Failures resolved during verification

The first PostgreSQL test attempts during implementation passed a complete URL where PostgreSQL's
database-name environment variable was expected; the client fell back to a local socket. The
helper was corrected to parse the URL into PostgreSQL environment fields, which also keeps
credentials out of process arguments, and the drill passed.

The first full-schema acceptance attempt used the database after application integration tests.
Those tests had committed object-reference fixtures whose temporary files had already been
removed. Backup correctly refused the inconsistent source. The drill was repeated against a
separately migrated, quiescent database and passed. No application business-rule change was
required.

## Not tested

No live S3-compatible bucket, provider-native versioning/retention lock, production credentials,
scheduled backup job, remote recovery host, remote CI or production deployment was tested.
Recovery-point and recovery-time objectives are therefore not claimed. Optional private
marketplace fixture tests were not enabled. Physical printer, camera and barcode hardware were
not tested. Destructive archival was intentionally not implemented or tested.

