# CommerceOps Architecture

CommerceOps is a modular monolith: one Go backend, one PostgreSQL database, and one Next.js frontend. Accepted ADRs and `MASTER_SPEC.md` govern foundational decisions.

## Repository boundaries

- `services/api`: primary Go application. `cmd/server` wires dependencies; infrastructure and domain packages live under `internal`.
- `apps/web`: strict-TypeScript Next.js frontend. It communicates with the backend only through REST APIs.
- `workers`: optional specialized processing only; it is not a second business backend.
- `services/api/migrations`: explicit, ordered PostgreSQL schema changes.

HTTP handlers translate protocol concerns and delegate substantive rules to owning modules. Database access must remain centralized and every future business-owned operation must be scoped to server-established tenant context.

## Phase 0 runtime

The API uses `net/http`, `log/slog`, and `pgx/v5`. Startup validates required configuration, creates a PostgreSQL pool without running migrations, and handles graceful shutdown. Dependency health is checked with a bounded timeout.

The frontend uses a typed API access layer. It does not connect to PostgreSQL or contain authoritative business logic.

REST endpoints are versioned under `/api/v1`. The OpenAPI document at `docs/openapi.yaml` is the contract source; generated clients may be introduced later when the API surface justifies them.

File storage is accessed only through the platform `objectstorage.Storage`
boundary. Local filesystem storage remains available for development and
tests; production can select the AWS SDK-backed S3-compatible implementation
for AWS S3, Cloudflare R2, MinIO, Backblaze B2, and equivalent SigV4 providers.
The application owns tenant authorization and server-generated tenant-scoped
keys; storage drivers own only object persistence. Python remains limited to
specialized workers.

## Background processing

PostgreSQL is the durable authority for marketplace processing jobs. Each
marketplace processor has a bounded worker pool. Every API process generates
random, non-secret worker identities and claims only queued or expired work for
that processor transactionally with `FOR UPDATE SKIP LOCKED`. A bounded lease
is renewed while extraction runs. Completion and failure transactions
must still own the live lease before they may persist results or final state,
so a stale process cannot commit after another instance reclaims its job.

The in-process wake channel is only a latency optimization. It is not a queue,
and no Redis, broker, or external workflow system participates in job
authority.

Shared marketplace orchestration owns upload validation, object storage,
tenant/marketplace source deduplication, job leases, retries, normalized
persistence, duplicate protection, Product Master lookup, and audits. Isolated
adapters under `internal/marketplace/<marketplace>` own document recognition,
field extraction, and marketplace-specific page association. The shared PDF
boundary may opt into bounded OCR for text-empty pages; Phase 7 enables that
capability only for Amazon. Amazon associates label and invoice pages by an
exact order ID and persists every contributing page through the generic
order-document traceability relation.
