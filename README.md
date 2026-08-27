# CommerceOps

CommerceOps is a modular ecommerce operations platform built as a Go and
PostgreSQL modular monolith with a strict-TypeScript Next.js frontend. See
[`docs/CURRENT_STATE.md`](docs/CURRENT_STATE.md) for the active phase,
implemented modules, review gates, and explicitly forbidden work.

## Prerequisites

- Go 1.24 or newer
- Node.js 22
- pnpm 11.19.0
- Docker with the Compose plugin
- GNU Make
- Poppler (`pdfinfo` and `pdftotext`) for PDF extraction

### Arch Linux

The following official Arch packages provide the required toolchain. Corepack
is used so pnpm matches the version pinned by this repository.

```bash
sudo pacman -S --needed go nodejs-lts-jod corepack docker docker-compose make poppler
sudo systemctl enable --now docker.service
corepack install --global pnpm@11.19.0
```

The package names are documented by Arch for
[`go`](https://archlinux.org/packages/extra/x86_64/go/),
[`nodejs-lts-jod`](https://archlinux.org/packages/extra/x86_64/nodejs-lts-jod/),
[`corepack`](https://archlinux.org/packages/extra/any/corepack/),
[`docker`](https://archlinux.org/packages/extra/x86_64/docker/),
[`docker-compose`](https://archlinux.org/packages/extra/x86_64/docker-compose/),
[`make`](https://archlinux.org/packages/core/x86_64/make/), and
[`poppler`](https://archlinux.org/packages/extra/x86_64/poppler/). Arch's
Poppler package includes both required command-line tools.

Docker commands require access to the Docker daemon. Use `sudo` or configure
Docker group access according to your local security policy; membership in the
Docker group is effectively root-level access.

## Local setup on Linux

From the repository root:

```bash
cp .env.example .env
# Replace every occurrence of the example database password in .env.
docker compose config
pnpm --dir apps/web install --frozen-lockfile
make dev-infra
make migrate
```

Start the backend and frontend in separate terminals:

```bash
make dev-backend
```

```bash
make dev-frontend
```

Open `http://localhost:3000`. The frontend uses the API at
`http://localhost:8080` by default.

The PostgreSQL schema is migration-owned. Application startup never applies
migrations automatically. `make migrate` uses the existing Compose migration
container and the local credentials in `.env`.

## Object storage

Local development defaults to `OBJECT_STORAGE_DRIVER=local` and stores files
under `FILE_STORAGE_DIR`. Production deployments can select the S3-compatible
driver:

```dotenv
OBJECT_STORAGE_DRIVER=s3
OBJECT_STORAGE_ENDPOINT=https://objects.example.com
OBJECT_STORAGE_BUCKET=commerceops
OBJECT_STORAGE_REGION=us-east-1
OBJECT_STORAGE_ACCESS_KEY=replace-with-deployment-secret
OBJECT_STORAGE_SECRET_KEY=replace-with-deployment-secret
OBJECT_STORAGE_PATH_STYLE=false
```

Leave `OBJECT_STORAGE_ENDPOINT` empty to use the standard AWS S3 regional
endpoint. Set the provider endpoint for Cloudflare R2, MinIO, or Backblaze B2,
and enable path-style addressing when that provider or local service requires
it. The bucket must already exist. Store credentials only in the deployment
secret system or uncommitted local `.env`; never commit them.

CommerceOps stores tenant ownership in PostgreSQL and generates tenant-scoped
object keys server-side. Object storage credentials do not replace application
authorization, and clients do not provide trusted tenant identifiers.

## Developer commands

```bash
make dev          # compatibility alias for dev-infra
make dev-infra    # start only PostgreSQL
make dev-backend  # run the Go API in the foreground
make dev-frontend # run Next.js in the foreground
make migrate      # apply migrations to the Compose PostgreSQL service
make test         # Go tests plus frontend type checking
make verify       # full checks; explicitly reports if PostgreSQL tests skip
make verify-full  # require and use TEST_DATABASE_URL; skipping is an error
make down         # stop the Compose environment
```

`make verify` is convenient during normal development. PostgreSQL-backed tests
run when `TEST_DATABASE_URL` is set and otherwise print an explicit skip
message. Phase-completion and remediation-completion claims must use an already
migrated disposable test database:

```bash
export TEST_DATABASE_URL='postgres://user:password@localhost:5432/commerceops_test?sslmode=disable'
make verify-full
```

## Optional Windows / PowerShell notes

Docker Desktop can provide Docker Compose on Windows. From PowerShell:

```powershell
Copy-Item .env.example .env
pnpm --dir apps/web install --frozen-lockfile
```

Replace the example password in `.env`, then use the Linux workflow from a
Make-capable environment such as WSL, Git Bash, or MSYS2. Native PowerShell can
run the backend and frontend in separate terminals with `go run ./cmd/server`
from `services/api` and `pnpm dev` from `apps/web`, but the Make targets remain
the canonical project commands.

## Project guidance

Read [`AGENTS.md`](AGENTS.md) before making changes. AI-assisted contributors
and reviewers must also follow
[`docs/AI_WORKFLOW.md`](docs/AI_WORKFLOW.md). Architecture, APIs, database
conventions, and security are documented under `docs/`.
