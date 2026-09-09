# Local Testing — Fedora Linux

## Goal

Run the approved Phase 14 app locally before beginning Phase 15 implementation.

## Confirm repository

```bash
git fetch --all
git checkout phase/14-printing-automation
git pull
git status
git log --oneline -5
```

Approved Phase 14 implementation commit:
`d90e4f7c0ceab032bc33a0618e1ceabdc20906b5`

## Check tools

```bash
go version
node --version
pnpm --version
docker --version
docker compose version
podman --version
make --version | head -1
pdfinfo -v
pdftotext -v
tesseract --version | head -1
```

Project prerequisites:
- Go 1.24+
- Node.js 22
- pnpm 11.19.0
- Docker + Compose
- GNU Make
- Poppler
- Tesseract English data for opt-in Amazon OCR

## Setup

```bash
cp .env.example .env
docker compose config
pnpm --dir apps/web install --frozen-lockfile
make dev-infra
make migrate
```

Never commit `.env`.

## Backend

```bash
make dev-backend
```

If environment variables are not loaded:

```bash
bash -c 'set -a; source .env; set +a; make dev-backend'
```

Expected API:
`http://localhost:8080`

## Frontend

```bash
make dev-frontend
```

Open:
`http://localhost:3000`

## First test order

1. login/bootstrap
2. Dashboard
3. Product Master
4. marketplace processing
5. Batch
6. artifacts
7. Inventory
8. Returns
9. Consignment
10. Print Library
11. Printer Agent
12. Quick Print
13. Automation

For every screen:

```text
✅ works
🟡 confusing
❌ broken
💡 improvement
```

Record commit, browser, user role, steps, expected, actual and screenshots.

## Role test users

Use fake/test users:
- Owner
- Developer/Test Admin
- HR
- Team Leader
- Worker

Verify backend denial, not only hidden UI.

## Myntra evidence test

Before Phase 11 completion:
- try Save to PDF / Print to File
- otherwise use controlled CUPS virtual-printer capture
- inspect raw captured payload privately
- sanitize fixture
- do not implement from photographs alone
