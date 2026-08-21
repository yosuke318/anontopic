.PHONY: help up down downd reset logs \
	build test lint fmt check \
	backend-build backend-run backend-test backend-lint backend-fmt \
	frontend-install frontend-build frontend-test frontend-lint frontend-format \
	load-test load-report

# 負荷テスト（Vegeta）の既定値。make load-test LOAD_RATE=200 のように上書きできる。
LOAD_RATE ?= 50
LOAD_DURATION ?= 30s
LOAD_TARGETS ?= test/load/targets.txt
LOAD_RESULT ?= test/load/results/latest.bin

help: ## Show available targets
	@grep -hE '^[^ 	#]+:.*## ' $(MAKEFILE_LIST) \
		| awk -F':' '{ target = $$1; sub(/.*## /, "", $$0); printf "  %-20s %s\n", target, $$0 }'

# --- ローカル環境 ---------------------------------------------------------

up: ## Start the local stack and wait until every service is healthy
	docker compose up -d --wait

down: ## Stop the local stack, keeping the PostgreSQL and Redis volumes
	docker compose down

downd: ## Stop the local stack and drop its data volumes
	docker compose down -v

reset: downd up ## Recreate the local stack from empty data volumes

logs: ## Follow the logs of every service
	docker compose logs -f

# --- 全体 -----------------------------------------------------------------

build: backend-build frontend-build ## Build the backend and the frontend

test: backend-test frontend-test ## Run the backend and frontend tests

lint: backend-lint frontend-lint ## Lint the backend and the frontend

fmt: backend-fmt frontend-format ## Format the backend and the frontend

check: lint test build ## Run every check required before pushing

# --- バックエンド（Go） ---------------------------------------------------

backend-build: ## Build all Go packages
	go build ./...

backend-run: ## Run the API server against the datastores started by `make up`
	go run ./cmd/server

backend-test: ## Run Go tests
	go test ./...

backend-lint: ## Run golangci-lint over the Go code
	golangci-lint run ./...

backend-fmt: ## Format Go code
	gofmt -w .

# --- フロントエンド（Next.js） --------------------------------------------

frontend-install: ## Install web dependencies
	cd web && npm install

frontend-build: ## Build the Next.js app
	cd web && npm run build

frontend-test: ## Run the web tests, if the package defines any
	cd web && npm test --if-present

frontend-lint: ## Lint the Next.js app
	cd web && npm run lint

frontend-format: ## Format the Next.js app with Prettier
	cd web && npm run format

# --- 負荷テスト -----------------------------------------------------------

load-test: ## Run a Vegeta load test against a locally running server
	@mkdir -p $(dir $(LOAD_RESULT))
	go tool vegeta attack -targets=$(LOAD_TARGETS) -rate=$(LOAD_RATE) -duration=$(LOAD_DURATION) -output=$(LOAD_RESULT)
	go tool vegeta report $(LOAD_RESULT)

load-report: ## Show the report and latency histogram of the last load test
	go tool vegeta report $(LOAD_RESULT)
	go tool vegeta report -type='hist[0,10ms,50ms,100ms,500ms,1s]' $(LOAD_RESULT)
