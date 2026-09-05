# Phase 14 printing automation

Automation owns only rule evaluation and logical execution. Every successful
execution creates an ordinary `printer_jobs` row through Printing's
transaction-scoped queue boundary. Agent authentication, claims, immutable PDF
downloads, physical delivery, and explicit physical retries remain in Printing.
Automation has no Inventory dependency.

## Rules and access

Grant `automations.view` for rules, previews, upcoming runs, execution history,
and derived reports. Grant `automations.manage` to create/edit/pause rules,
change company timezone, test, and retry queue failures. These permissions are
not automatically granted to existing roles. The editor's options endpoint
provides friendly tenant asset/printer selections under `automations.manage`;
it exposes no OS identifiers and needs no printer-administration grant.

Rules select an existing Print Library PDF and registered printer. Copies are
1–100; an optional daily limit is between the configured copies and 10,000.
There are at most 100 rules per company to bound event fan-out. Edits require
the current version and append before/after snapshots to the audit log. Runs
retain their original rule, asset, printer, copies, timezone, and version.

The only triggers are `scheduled`, `ecommerce_batch_ready`,
`consignment_packing`, and `consignment_packed`. Event rules have an empty
schedule configuration. Their asset is explicitly configured: batch readiness
never guesses a generated artifact or starts PDF generation.

## Time and restart semantics

Companies have an explicit IANA timezone, defaulting to UTC. The timezone is
snapshotted on rule creation/edit; changing company timezone alone never
changes existing schedules. Daily, weekdays, selected ISO weekdays (1–7), up
to 24 distinct HH:MM times, and inclusive optional start/end dates are supported.
Preview uses the same calculation as execution. Missing DST minutes are
skipped; repeated minutes use the first instant once, including non-hour DST.

A schedule occurrence key is local date plus clock time within its rule. It
intentionally does not include version: editing cannot reprint an occurrence
already materialized that day. Persisted overdue occurrences catch up in order
in bounded ticks after downtime. Pausing/disabling prevents new materialization;
already materialized automatic work is skipped if it reaches the worker while
the rule is inactive. Resuming or editing computes a new future next run.

Batch and Consignment insert durable events in the same transaction as the
first successful state transition. Replayed transitions cannot duplicate an
event. The consumer matches only enabled, unpaused rules activated before the
event occurred. Editing/reactivating a rule does not apply its new configuration
to older, unconsumed facts. Events and executions remain available as evidence.

Each API process runs one bounded worker loop. A tick handles at most 32 due
occurrences, 32 events, and 32 executions. PostgreSQL row locks, `SKIP LOCKED`,
unique occurrence keys, one-minute execution leases, and fenced completion
support concurrent processes and restart. Shutdown cancels the worker and waits
for it before closing the database pool. No broker or external cron is needed.

## Failures and safety

The rule lock serializes execution and derived daily copy accounting. Every
normal automation job, including an explicit physical retry, counts toward the
rule's local-day budget; cancelled/failed jobs count conservatively. Test runs
also count. Limit exhaustion is a visible skipped execution.

Offline/disabled printers and inactive assets fail visibly. Repeated queue
failures back off exponentially from the configured 1–3,600 seconds, capped at
one hour. The configured 1–20 failure threshold pauses the rule and audits the
pause. Resume resets consecutive failure/backoff state. Operational database
errors are logged, and their leased work is reclaimable after expiry.

Queue creation, the Printing queued event/audit, and the execution result commit
in one transaction. An interrupted transaction leaves no partial job. The
execution UUID also supplies Printing's semantic idempotency key. Retry of an
execution that already produced a job is a no-op, even if that job subsequently
failed physically. Inspect/retry physical failures through Printing's explicit
reprint workflow; automation never resubmits potentially printed work.

A test run is an explicit real print request, including for disabled/paused
rules. It respects daily limits and failure backoff. The UI confirms the asset,
printer, and copies. Reusing its request key returns the same execution and
original snapshot; the client retains the key across uncertain HTTP failures.

## Workspace and reporting

Rules, Scheduled Prints, Upcoming, Recent Runs, Failures, and Reporting are
available in the responsive workspace. History includes rule changes, pause
records, execution snapshots, errors, and links to physical job identity. Views
refresh every 15 seconds. Recent runs/failures are bounded to the latest 200.

`completed` on an execution means job creation succeeded; `job_status` describes
physical printing. All-time manual/automatic totals, copies, completed/failed/
pending/cancelled outcomes, and failure-event counts derive from `printer_jobs`
and append-only `printer_job_events`. No reporting counter tables exist.

## Migration and verification

Migration `000022` adds company timezone, automation rules, durable domain
events, executions, permissions, indexes, tenant-composite foreign keys, and the
`automation` print origin. It does not alter inventory. Apply before starting
the Phase 14 server. A downgrade refuses to discard existing automation print
job origins; use down/up only in a disposable database without such jobs.

Run `TEST_DATABASE_URL=... make verify-full` against an already migrated
disposable PostgreSQL database. Automation tests use isolated schemas and cover
schedules/DST, leases, concurrency, transaction rollback, limits, failures,
permissions/tenants, history, ordinary agent delivery, migration, and inventory
neutrality. Batch and Consignment tests exercise authoritative event hooks.
