SHELL := /bin/sh

.PHONY: dev migrate test verify down \
	backend-format backend-vet backend-test backend-build \
	frontend-typecheck frontend-lint frontend-build repository-check

dev:
	docker compose up -d postgres
	@echo "PostgreSQL is running. In separate terminals run:"
	@echo "  cd services/api && go run ./cmd/server"
	@echo "  cd apps/web && pnpm dev"

migrate:
	docker compose --profile tools run --rm migrate

test: backend-test frontend-typecheck

verify: backend-format backend-vet backend-test backend-build frontend-typecheck frontend-lint frontend-build repository-check

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
