SHELL := /bin/sh

.PHONY: dev dev-infra dev-backend dev-frontend migrate test verify verify-full down \
	backend-format backend-vet backend-test backend-build \
	frontend-typecheck frontend-lint frontend-build repository-check

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

migrate:
	@test -f .env || { echo ".env is required; copy .env.example and set a local password"; exit 1; }
	@set -a; . ./.env; set +a; \
	docker compose --profile tools run --rm migrate \
		-path=/migrations \
		-database="postgres://$${COMMERCEOPS_POSTGRES_USER}:$${COMMERCEOPS_POSTGRES_PASSWORD}@postgres:5432/$${COMMERCEOPS_POSTGRES_DB}?sslmode=disable" \
		up

test: backend-test frontend-typecheck

verify: backend-format backend-vet backend-test backend-build frontend-typecheck frontend-lint frontend-build repository-check

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
	cd services/api && go build ./cmd/server

frontend-typecheck:
	cd apps/web && pnpm typecheck

frontend-lint:
	cd apps/web && pnpm lint

frontend-build:
	cd apps/web && pnpm build

repository-check:
	git diff --check

down:
	docker compose down
