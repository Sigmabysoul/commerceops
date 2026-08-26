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

File storage will be S3-compatible when a later phase requires it. Python remains limited to specialized workers.
