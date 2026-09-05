# Phase 14 Printing Automation Design

Status: implementation authorized by the owner on 2026-09-05.

Phase 14 creates ordinary Phase 13 `printer_jobs`; it never sends documents to
agents or printers directly. Automation has no Inventory dependency and cannot
turn a print event into stock movement.

## Domain model

Migration `000022` will add:

- `companies.timezone`: explicit IANA timezone, initially `UTC`, validated by
  the Go timezone database before changes are accepted.
- `automation_rules`: tenant, name, enabled/paused state, approved trigger type,
  trigger configuration, immutable Print Library asset strategy, registered
  printer, copies, optional daily limit, failure threshold/backoff, creator,
  optimistic version, next run, and timestamps.
- `automation_domain_events`: durable tenant events recorded transactionally by
  Batch or Consignment when their authoritative state transition commits.
- `automation_executions`: one logical rule occurrence, its unique occurrence
  key, schedule/event evidence, lease, status, error, resulting normal
  `printer_job`, and timestamps.

Rules initially support only:

- `scheduled`
- `ecommerce_batch_ready`
- `consignment_packing`
- `consignment_packed`

No UI observation, inferred state, generic webhook, arbitrary SQL/event name,
or new business-domain trigger is allowed.

## Schedule contract

Scheduled rules store structured JSON with:

- mode: `daily`, `weekdays`, or `selected_weekdays`
- one or more `HH:MM` local clock times
- selected ISO weekdays when required (`1` Monday through `7` Sunday)
- optional inclusive local start and end dates

The rule snapshots the explicit company IANA timezone. Next-run preview and
execution use the same pure calculation. A local occurrence key combines rule,
local date, and local clock time, so restarts and the repeated DST fall-back
hour still execute it once. A nonexistent spring-forward clock time is skipped,
not shifted to an unexpected time. Changing the company timezone does not
silently rewrite existing rules; an authorized rule update explicitly adopts
the new zone and increments its version.

## Durable scheduler

PostgreSQL is the scheduler authority; there is no broker or external cron.
Every API process may run the bounded scheduler loop:

1. Materialize due schedule occurrences using row locks and unique occurrence
   keys, then advance `next_run_at` transactionally.
2. Convert unprocessed authoritative domain events into matching logical
   executions with a unique `(rule,event)` occurrence.
3. Claim pending/expired executions with `FOR UPDATE SKIP LOCKED` and a lease.
4. Create a normal Phase 13 `printer_job` using the execution ID as its semantic
   idempotency key.
5. Link the execution to that job and mark it completed, or record bounded
   failure/backoff state.

Restarting at any step replays the same unique occurrence and printer-job
idempotency key. A completed logical occurrence cannot produce a second job.

## Authoritative event boundary

A tiny shared event-recorder contract accepts the caller's active PostgreSQL
transaction. Batch records `ecommerce_batch_ready` only in the transaction that
first changes a batch to ready. Consignment records `consignment_packing` or
`consignment_packed` only in the transaction that performs that versioned state
change. Replayed transitions do not create another event.

The recorder persists facts only; it does not evaluate rules or create jobs.
The Automation domain consumes them later. This preserves domain ownership and
ensures an event cannot exist without its source transition, or be lost after
that transition commits.

## Asset strategy

Phase 14 event and scheduled rules print an explicitly configured active Print
Library PDF. This gives the execution an immutable Phase 13 source at trigger
time. It does not guess which marketplace-generation artifact might exist when
a batch becomes ready. A future generated-artifact strategy requires a separate
approved trigger whose authoritative event occurs after artifact generation.

## Safety and failures

- Dedicated `automations.view` and `automations.manage` permissions.
- Rule/query/mutation scope always comes from the authenticated tenant.
- Copies remain within Phase 13's 1–100 bound.
- Optional daily limits count derived completed/queued jobs, not a mutable
  counter. Limit exhaustion records a visible skipped execution.
- Offline/disabled printers cause a visible failed execution; they never cause
  direct hardware fallback.
- Consecutive failures apply bounded exponential backoff. Reaching the chosen
  threshold pauses the rule and creates an audit record.
- Test run is an explicit audited execution and creates a normal job.
- Rule edits use optimistic `version` checks and preserve execution history.
- Manual pause/disable prevents future materialization but never deletes runs.
- Retrying an execution reuses its logical occurrence/idempotency identity and
  cannot duplicate a successfully created physical job.

## API and UX

Session-authenticated APIs will provide:

- company timezone read/update
- rule list/create/read/update/pause
- schedule next-run preview
- explicit test run
- upcoming executions
- recent runs/failures
- derived automation versus manual and printer reliability summaries

The responsive Automation workspace separates Rules, Scheduled Prints,
Upcoming, Recent Runs, and Failures. It displays friendly printer and asset
names, local timezone, next run, copies, enabled/paused state, and failure
reason. It never accepts a system printer identifier or arbitrary trigger name.

## Reporting

Reports derive automatic/manual counts, copies, outcome, and printer reliability
from `printer_jobs`, linked executions, and append-only job events. No reporting
counter table is introduced.

## Verification plan

- IANA timezone validation, daily/weekday/selected-day calculation, start/end
  dates, DST skipped/repeated time behavior, and next-run preview parity.
- Restart and concurrent scheduler exact-once materialization/job creation.
- Disabled, paused, date-expired, daily-limit, offline-printer, backoff, and
  auto-pause behavior.
- Permission and tenant isolation for rules, runs, events, and reports.
- Transactional batch-ready and consignment packing/packed event capture.
- Test run, failure, retry, audit, optimistic version conflict, and history.
- Ordinary Phase 13 agent delivery for automatically created jobs.
- Inventory ledger and balances unchanged across every automation operation.
- Migration up/down in an isolated schema, Go tests/vet/build, frontend
  typecheck/lint/build, OpenAPI parse, and full PostgreSQL-backed verification.
