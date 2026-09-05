COMMERCEOPS — PHASE 0: PROJECT FOUNDATION

ROLE

You are the primary implementation engineer for CommerceOps.

CommerceOps is intended to become a long-lived ecommerce operations platform and potentially a multi-company SaaS product.

Treat this as production-oriented software, not a disposable prototype.

DO NOT proceed to any later phase automatically.


==================================================
MANDATORY READING
==================================================

Before changing code, read:

- AGENTS.md
- docs/MASTER_SPEC.md
- docs/ARCHITECTURE.md
- docs/DOMAIN_RULES.md
- docs/CURRENT_STATE.md
- docs/ROADMAP.md
- docs/phases/PHASE-00-FOUNDATION.md if it exists

Also read nested AGENTS.md files for directories you modify.

If documentation conflicts with implementation, report the conflict instead of silently deciding which one to follow.


==================================================
APPROVED STACK
==================================================

Frontend:
- TypeScript
- React
- Next.js
- strict TypeScript

Backend:
- Go

Database:
- PostgreSQL

API:
- REST
- versioned under /api/v1

Architecture:
- modular monolith

File storage later:
- S3-compatible object storage

Do not change these decisions.


==================================================
PHASE GOAL
==================================================

Create a clean engineering foundation on which the real CommerceOps business modules can safely be built.

Phase 0 contains infrastructure only.

Do NOT implement actual ecommerce/business features.


==================================================
REQUIRED REPOSITORY AREAS
==================================================

Expected high-level structure:

commerceops/
├── apps/
│   └── web/
├── services/
│   └── api/
├── workers/
├── packages/
├── docs/
├── infra/
├── scripts/
└── .github/


Backend direction:

services/api/
├── cmd/
│   └── server/
├── internal/
├── migrations/
├── tests/
├── go.mod
└── go.sum


Frontend direction:

apps/web/
├── src/
│   ├── app/
│   ├── components/
│   ├── features/
│   ├── api/
│   ├── lib/
│   └── types/
├── package.json
└── tsconfig.json


Do not create speculative business modules just to fill folders.


==================================================
IMPLEMENT
==================================================

1. Go Backend Foundation

Create a Go application that:

- starts cleanly
- uses a clear configuration system
- has structured logging
- has graceful shutdown
- has consistent API response/error conventions
- keeps main.go focused on dependency wiring/startup
- does not put business logic in main.go
- can connect to PostgreSQL


2. Configuration

Use environment variables.

Create:

.env.example

with placeholders only.

Never commit actual:

- passwords
- API keys
- tokens
- database secrets

Startup should clearly fail when required configuration is missing.


3. PostgreSQL Development Environment

Provide a simple local PostgreSQL setup.

Docker Compose is acceptable.

Docker Compose is supported but optional.

Native PostgreSQL is also a supported local development environment.

Requirements:

- persistent development volume where appropriate
- health check where useful
- environment-driven credentials
- no production credentials
- documented startup instructions


4. Database Migration System

Establish a migration strategy for future schema changes.

Do NOT create the entire CommerceOps database.

Only infrastructure-level migration setup is required.

Every future schema modification must happen through migrations.


5. Health API

Implement:

GET /api/v1/health

The endpoint should verify basic application health.

It should indicate whether required dependencies such as PostgreSQL are reachable.

Do not expose:

- passwords
- host secrets
- stack traces
- internal credentials

Example conceptual response:

{
  "status": "ok",
  "database": "ok"
}


6. Frontend Foundation

Create the Next.js frontend using strict TypeScript.

Requirements:

- production build passes
- lint/typecheck passes
- clear project organization
- API access layer exists
- frontend can call GET /api/v1/health
- simple development health page/component

Do NOT create the real CommerceOps dashboard.


7. API Access

Frontend must communicate with the Go backend through the API.

Frontend must NOT:

- connect directly to PostgreSQL
- contain authoritative business logic
- act as a second backend


8. CORS / Development Connectivity

Set up only what is needed for local frontend/backend communication.

Avoid insecure wildcard production configuration.


9. Logging

Use a simple structured logging foundation.

Logs should support useful fields such as:

- timestamp
- level
- request context when applicable
- error

Do not overbuild observability.


10. API Error Convention

Establish a consistent error response.

For example conceptually:

{
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "Something went wrong"
  }
}

Do not expose internal SQL/database messages directly to frontend users.


11. Development Commands

Document commands for:

- start local dependencies
- start backend
- start frontend
- run migrations
- run backend tests
- run frontend checks
- build backend
- build frontend


12. CI

Create GitHub Actions appropriate for the repository.

Backend checks should include applicable:

- gofmt verification
- go vet
- go test
- go build

Frontend checks should include applicable:

- dependency installation
- lint
- typecheck
- tests if configured
- production build

CI must use commands that actually exist.


13. Basic Tests

Create meaningful tests for Phase 0 infrastructure where appropriate.

Avoid useless tests purely to inflate test count.


14. Documentation

Update:

- README.md
- docs/CURRENT_STATE.md
- docs/ARCHITECTURE.md if needed
- API/setup documentation as required

CURRENT_STATE.md must clearly say Phase 0 is implemented but awaiting review.


==================================================
DO NOT IMPLEMENT
==================================================

Do not implement:

- companies
- real authentication
- employees
- permissions
- Product Master
- SKU mapping
- Flipkart processing
- Amazon
- Meesho
- Myntra
- Snapdeal
- batches
- inventory
- returns
- consignment
- billing
- printer agent
- AI/OCR
- Redis
- Kafka
- RabbitMQ
- Kubernetes
- Elasticsearch
- microservices


==================================================
DEPENDENCY RULE
==================================================

Prefer the Go standard library when practical.

Before adding any dependency:

1. determine why it is necessary
2. check whether an existing dependency solves it
3. avoid large frameworks when a small library is sufficient

List every added dependency in the completion report.


==================================================
IMPLEMENTATION PROCESS
==================================================

Before modifying code:

1. inspect the existing repository
2. compare it with Phase 0 requirements
3. produce a concise implementation plan
4. identify files to create/change
5. identify dependencies
6. identify architectural conflicts

Then implement.


==================================================
DEFINITION OF DONE
==================================================

Phase 0 is implementation-complete only if:

- Go backend builds
- Next.js frontend builds
- strict TypeScript passes
- PostgreSQL local development works
- migration tooling works
- backend connects to PostgreSQL
- /api/v1/health works
- frontend calls the health endpoint
- relevant backend tests pass
- frontend checks pass
- CI is configured
- no secrets are committed
- documentation is updated
- no Phase 1 business functionality was introduced


==================================================
COMPLETION REPORT
==================================================

At the end provide:

1. Files created/changed
2. Architecture established
3. Dependencies added and why
4. Commands executed
5. Tests executed
6. Test/build results
7. Known problems
8. Documentation updated
9. Architecture concerns
10. Anything intentionally postponed

Finish with exactly one:

PHASE 0 IMPLEMENTATION COMPLETE — READY FOR EXTERNAL REVIEW

or

PHASE 0 NOT COMPLETE

STOP after Phase 0.
Do not begin Phase 1.