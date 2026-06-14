.PHONY: help up down logs backend-run backend-tidy backend-test backend-test-integration \
        frontend-dev frontend-build migrate-up migrate-down migrate-create

# Override with your own DSN if needed.
DATABASE_URL ?= postgres://app:app@localhost:5432/ai_data_marketplace?sslmode=disable
MIGRATIONS_DIR := backend/migrations

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up: ## Start the full local stack (postgres, redis, backend, frontend)
	docker compose up --build

down: ## Stop and remove the local stack
	docker compose down

logs: ## Tail stack logs
	docker compose logs -f

backend-run: ## Run the backend locally (needs Go + local postgres/redis)
	cd backend && go run ./cmd/api

backend-tidy: ## Resolve/lock Go dependencies (generates go.sum)
	cd backend && go mod tidy

backend-test: ## Run backend tests
	cd backend && go test ./...

backend-test-integration: ## Run pg-backed concurrency integration tests (needs a real Postgres)
	cd backend && TEST_DATABASE_URL="$(DATABASE_URL)" go test -tags=integration -run TestConcurrent ./...

migrate-up: ## Apply all up migrations (needs `migrate` CLI: golang-migrate)
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

migrate-create: ## Scaffold a new migration: make migrate-create name=add_foo
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

frontend-dev: ## Run the frontend dev server
	cd frontend && npm run dev

frontend-build: ## Production build of the frontend
	cd frontend && npm run build
