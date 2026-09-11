SHELL := /bin/sh

.PHONY: dev dev-infra dev-backend dev-frontend local-launcher migrate test verify verify-full down \
	backend-format backend-vet backend-test backend-build \
	frontend-typecheck frontend-lint frontend-build repository-check lifecycle-test \
	lifecycle-backup lifecycle-validate lifecycle-restore lifecycle-archive-plan

dev: dev-infra

dev-infra:
	docker compose up -d postgres
	@echo "PostgreSQL is running. In separate terminals run:"
	@echo "  make dev-backend"
	@echo "  make dev-frontend"

dev-backend:
	cd services/api && go run ./cmd/server

dev-frontend:
	cd apps/web && pnpm dev

local-launcher:
	cd services/api && go build -o ../../CommerceOps ./cmd/local-launcher

migrate:
	@test -f .env || { echo ".env is required; copy .env.example and set a local password"; exit 1; }
	@set -a; . ./.env; set +a; \
	docker compose --profile tools run --rm migrate \
		-path=/migrations \
		-database="postgres://$${COMMERCEOPS_POSTGRES_USER}:$${COMMERCEOPS_POSTGRES_PASSWORD}@postgres:5432/$${COMMERCEOPS_POSTGRES_DB}?sslmode=disable" \
		up

test: backend-test frontend-typecheck

verify: backend-format backend-vet backend-test backend-build frontend-typecheck frontend-lint frontend-build lifecycle-test repository-check

verify-full:
	@if [ -z "$${TEST_DATABASE_URL:-}" ]; then \
		echo "TEST_DATABASE_URL is required for verify-full"; \
		exit 1; \
	fi
	@$(MAKE) verify

backend-format:
	@files="$$(cd services/api && gofmt -l .)"; \
	if [ -n "$$files" ]; then echo "gofmt required:"; echo "$$files"; exit 1; fi

backend-vet:
	cd services/api && go vet ./...

backend-test:
	@if [ -n "$(TEST_DATABASE_URL)" ]; then \
		echo "PostgreSQL integration tests: ENABLED via TEST_DATABASE_URL"; \
	else \
		echo "PostgreSQL integration tests: SKIPPED (TEST_DATABASE_URL is not set)"; \
	fi
	cd services/api && go test ./... -count=1

backend-build:
	cd services/api && go build ./cmd/server ./cmd/printer-agent ./cmd/local-launcher

frontend-typecheck:
	cd apps/web && pnpm typecheck

frontend-lint:
	cd apps/web && pnpm lint

frontend-build:
	cd apps/web && pnpm build

repository-check:
	git diff --check

lifecycle-test:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/lifecycle/test_lifecycle.py -v

lifecycle-backup:
	@test -n "$(BACKUP_DIR)" || { echo "BACKUP_DIR is required"; exit 1; }
	@test -n "$(OBJECT_ROOT)" || { echo "OBJECT_ROOT is required"; exit 1; }
	PYTHONDONTWRITEBYTECODE=1 python3 scripts/lifecycle/lifecycle.py backup "$(BACKUP_DIR)" --object-root "$(OBJECT_ROOT)"

lifecycle-validate:
	@test -n "$(BACKUP_DIR)" || { echo "BACKUP_DIR is required"; exit 1; }
	PYTHONDONTWRITEBYTECODE=1 python3 scripts/lifecycle/lifecycle.py validate "$(BACKUP_DIR)"

lifecycle-restore:
	@test -n "$(BACKUP_DIR)" || { echo "BACKUP_DIR is required"; exit 1; }
	@test -n "$(RESTORE_OBJECT_DIR)" || { echo "RESTORE_OBJECT_DIR is required"; exit 1; }
	@test -n "$(RESTORE_RECEIPT)" || { echo "RESTORE_RECEIPT is required"; exit 1; }
	PYTHONDONTWRITEBYTECODE=1 python3 scripts/lifecycle/lifecycle.py restore "$(BACKUP_DIR)" \
		--object-destination "$(RESTORE_OBJECT_DIR)" --receipt "$(RESTORE_RECEIPT)"

lifecycle-archive-plan:
	@test -n "$(BACKUP_DIR)" || { echo "BACKUP_DIR is required"; exit 1; }
	@test -n "$(RESTORE_RECEIPT)" || { echo "RESTORE_RECEIPT is required"; exit 1; }
	@test -n "$(ARCHIVE_BEFORE)" || { echo "ARCHIVE_BEFORE is required"; exit 1; }
	@test -n "$(ARCHIVE_PLAN)" || { echo "ARCHIVE_PLAN is required"; exit 1; }
	PYTHONDONTWRITEBYTECODE=1 python3 scripts/lifecycle/lifecycle.py plan-archive "$(BACKUP_DIR)" \
		--restore-receipt "$(RESTORE_RECEIPT)" --before "$(ARCHIVE_BEFORE)" --output "$(ARCHIVE_PLAN)"

down:
	docker compose down
