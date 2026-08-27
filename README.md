# CommerceOps

CommerceOps is a modular ecommerce operations platform. Phase 0 provides the Go API, PostgreSQL development environment, explicit migration tooling, strict-TypeScript Next.js frontend, and CI foundation. Business modules have not been implemented.

## Prerequisites

- Go 1.24+
- Docker with Docker Compose
- Node.js 22+
- pnpm 11.19+
- Poppler (`pdfinfo` and `pdftotext`) for Phase 3 PDF extraction

## Local setup

```powershell
Copy-Item .env.example .env
# Replace the local placeholder password in .env before continuing.
docker compose config
docker compose up -d postgres


```

Local PostgreSQL can be run either:

1. natively
2. through Docker Compose

Environment variables are documented in `.env.example`. `DATABASE_URL` is required by the API. Do not commit `.env`.

Start the backend:

```powershell
Set-Location services/api
go run ./cmd/server
```

Start the frontend in another terminal:

```powershell
Set-Location apps/web
pnpm install
pnpm dev
```

Open `http://localhost:3000`. The development page calls `GET http://localhost:8080/api/v1/health`.

## Migrations

Migrations are explicit and never run during application startup. After adding a genuine schema migration under `services/api/migrations`, apply pending migrations with:

```powershell
docker compose --profile tools run --rm migrate
```

Phase 0 intentionally has no SQL migration because it has no schema requirement.

## Checks

```powershell
Set-Location services/api
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/server

Set-Location ../../apps/web
pnpm lint
pnpm typecheck
pnpm build
```

See `docs/ARCHITECTURE.md`, `docs/API.md`, `docs/DATABASE.md`, and `docs/SECURITY.md` for foundation conventions.
