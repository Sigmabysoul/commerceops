# Phase 13 Printing Platform Design

Status: approved for implementation by the owner on 2026-09-04.

This design precedes Phase 13 code. It preserves the existing marketplace PDF
generation pipeline and introduces one canonical domain for physical delivery.
Printing and reprinting are inventory-neutral by construction: the Printing
domain has no Inventory service dependency and no inventory table writes.

## 1. Domain and schema design

Existing `print_jobs` and `print_artifacts` continue to represent generation of
immutable marketplace PDFs. They are not renamed or redefined. A new
`printer_jobs` aggregate represents every physical print request.

- `printer_agents`: tenant-owned device identity, friendly name, status,
  platform, last-seen timestamp, revocation state, and capability metadata.
- `printer_agent_credentials`: one-time-issued high-entropy credentials stored
  only as hashes, scoped to one tenant and agent, with expiry/revocation and
  last-used timestamps.
- `registered_printers`: tenant and agent ownership, friendly CommerceOps name,
  opaque OS printer identifier, capabilities, optional location, enabled state,
  reported availability, and last-seen timestamp. The OS identifier is never
  accepted as part of a print request.
- `print_library_assets`: tenant-owned metadata plus a server-generated object
  storage key, immutable PDF hash/size/page count, optional tenant-valid Product
  Master association, default printer/copies, favorite and active flags.
- `printer_jobs`: tenant, requester, registered printer, exactly one immutable
  source (`print_artifact_id` or `print_library_asset_id`), copies, origin type
  and reference, lifecycle state, idempotency key/request hash, lease fields,
  timestamps, bounded retry information, and failure details.
- `printer_job_events`: append-only lifecycle/audit detail for queued, claimed,
  printing, completed, failed, cancelled, lease-expired, and explicit retry
  events.

All compound foreign keys include `company_id`. User idempotency is unique per
tenant. Source columns use an XOR constraint. Copy counts are positive and
bounded. Agent claims use `FOR UPDATE SKIP LOCKED`, a random lease token, and a
finite expiry. Only the owning agent can claim jobs for its enabled printers.

An explicit retry creates a new attempt for the same immutable source and
printer; it does not silently re-execute a possibly printed attempt. Phase 13
accepts origins `ecommerce_batch`, `ecommerce_reprint`, `consignment`, and
`quick_print`. `scheduled` and `automation` remain reserved for Phase 14 and are
rejected by Phase 13 service methods.

## 2. Agent protocol

The local agent uses HTTPS JSON APIs; browser sessions are not accepted on
agent routes.

1. An authorized manager creates an agent and receives a credential once.
2. The agent authenticates with `Authorization: Bearer <credential>`.
3. It registers/reconciles locally enumerated printers and sends heartbeats.
4. It long-polls for one eligible job. The server atomically returns the job,
   immutable artifact metadata, a bounded download route, and a lease token.
5. The agent downloads through that authenticated route, verifies size and
   SHA-256, and records the job ID durably before invoking the OS adapter.
6. It reports `printing`, then `completed` or `failed`, using the lease token.
7. Repeated reports are idempotent. Stale or foreign lease tokens are rejected.

The agent maintains a durable local journal keyed by server job ID. Once an ID
has reached the OS-submission boundary, reconnects never submit it again. This
chooses at-most-once physical submission: a crash at the boundary can require
operator reconciliation, but automatic recovery cannot produce a duplicate.
An operator may create an audited explicit retry after checking the printer.

## 3. Security threat model

- Credential theft: secrets are random, returned once, hashed at rest,
  expiration-capable, revocable, agent-scoped, and compared in constant time.
- Cross-tenant access: tenant comes only from the authenticated user or agent;
  every query and foreign key is tenant-scoped.
- Arbitrary local execution: requests contain only a registered printer UUID,
  copies, and an artifact. No command, path, option list, or OS identifier is
  accepted from a print job.
- Malicious PDFs: uploads require PDF signature, bounded size, structural page
  validation, immutable hashing, and object storage under server-generated
  tenant keys. The agent verifies the downloaded hash before printing.
- Storage-key disclosure: clients never submit or receive raw storage keys.
- Replay/races: request hashes protect idempotency keys; leases, state guards,
  local journaling, and append-only events protect delivery and reporting.
- Disabled devices: revoked agents/credentials and disabled/offline printers
  cannot claim new work.
- Data leakage in logs: credentials, PDF content, storage keys, and customer
  document data are excluded from logs and audit metadata.
- Transport: production agent endpoints require TLS at the deployment edge.

## 4. Linux/CUPS boundary

The first OS adapter targets Linux/CUPS. It is a narrow interface for printer
discovery and PDF submission. The implementation invokes fixed CUPS programs
directly without a shell, validates copy bounds, uses only OS identifiers
obtained from discovery and reconciled by the server, and prints only a
server-downloaded managed temporary PDF. API input can never select an
executable, filesystem path, shell text, or arbitrary CUPS option.

The agent core owns authentication, polling, hash verification, the durable
journal, and status reporting. The CUPS adapter owns only discovery and one
submission call. It has no business-domain or Inventory access.

## 5. Future Windows compatibility

The agent core depends on a `PrinterBackend` interface with `List` and
`SubmitPDF` operations. CUPS is selected on Linux. A future Windows adapter can
use the Windows spooler behind the same interface without changing server
schema, protocol, job semantics, or mobile UI. Platform capability metadata is
opaque versioned JSON; server logic relies only on common normalized fields.

## 6. Mobile UX plan

The responsive Quick Print screen shows large accessible cards, favorites
first, category filters, search, and recent activity. Selecting an asset opens
a confirmation sheet with its default printer/copies. Workers select only
friendly CommerceOps printer names. Copies are bounded server-side; quantities
above the configured warning threshold require an explicit confirmation flag.
Offline/disabled printers are visible but cannot be selected. Success returns a
trackable queue state; the phone never contacts CUPS or a printer directly.

Separate permission-gated management views cover agents, printer names and
locations, library metadata/assets, credential rotation/revocation, job status,
cancellation, and explicit retries.

## 7. Migration and API plan

Migration `000021` creates the six Phase 13 tables, tenant indexes and state
constraints, and inserts these permissions:

- `printers.view`, `printers.manage`
- `printing.print`, `printing.reprint`
- `print_library.view`, `print_library.manage`

User APIs under `/api/v1` manage agents, credentials, printers, library assets,
physical jobs, cancellation/retry, and tenant-authorized downloads. Existing
batch artifact download and generation routes remain unchanged; a new action
queues an existing artifact into the physical printing domain.

Agent APIs under `/api/v1/printer-agent` cover heartbeat/reconciliation, claim,
artifact download, and guarded state reports. They use dedicated agent
authentication middleware and never accept browser authorization as an agent
identity. Contracts are documented in OpenAPI.

## 8. Test plan

- Migration up/down object, constraint, permission, and cleanup verification.
- Tenant isolation and every dedicated permission boundary.
- One-time credential issuance, hash-at-rest, expiry/revocation, malformed and
  cross-agent authentication.
- Printer registration/reconciliation, friendly-name selection, heartbeat,
  offline calculation, enable/disable behavior, and ownership.
- Library upload size/signature/structure checks, server-owned keys, metadata,
  Product Master ownership, defaults, archive, and cross-tenant denial.
- Quick-print copy guardrails, large-copy confirmation, favorites/categories,
  recent history, semantic request hashing, and concurrent idempotency.
- Existing batch and reprint artifact queueing through the same physical job.
- Atomic claims, lease ownership/expiry, reconnect replay, durable local journal,
  failed printers, explicit retry, cancellation, and concurrent agents.
- Lifecycle audit records and append-only job events.
- Inventory ledger and balances unchanged by generation, queueing, printing,
  completion, failure, retry, cancellation, and reprint.
- CUPS command construction tests prove fixed executable/arguments and reject
  unknown printer IDs, invalid copies, paths, and options.
- Go formatting/vet/tests/build, agent build, frontend typecheck/lint/build,
  OpenAPI parse, repository checks, and full PostgreSQL-backed verification.

Phase 13 stops at operator-requested printing. It introduces no schedules,
timers that create jobs, automation rules, or automatic print triggers.
