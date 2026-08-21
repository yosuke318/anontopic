.PHONY: help hooks up down downd reset logs migrate seed \
	build test lint fmt check \
	backend-build backend-run backend-test backend-lint backend-fmt \
	frontend-install frontend-build frontend-test frontend-lint frontend-typecheck frontend-format \
	load-test load-report

# compose と同じ .env を読む。読まないと、ポートを変えている環境で migrate や seed が
# 別のデータベースに接続してしまう。
-include .env

# ホスト側から DB につなぐときの接続先。compose が公開するポートに合わせる。
POSTGRES_PORT ?= 5432
POSTGRES_USER ?= anontopic
POSTGRES_PASSWORD ?= anontopic
POSTGRES_DB ?= anontopic
LOCAL_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable

# 負荷テスト（Vegeta）の既定値。make load-test LOAD_RATE=200 のように上書きできる。
LOAD_RATE ?= 50
LOAD_DURATION ?= 30s
LOAD_TARGETS ?= test/load/targets.txt
LOAD_RESULT ?= test/load/results/latest.bin

help: ## Show available targets
	@grep -hE '^[^ 	#]+:.*## ' $(MAKEFILE_LIST) \
		| awk -F':' '{ target = $$1; sub(/.*## /, "", $$0); printf "  %-20s %s\n", target, $$0 }'

hooks: ## Point git at the repository's hooks in .githooks
	git config core.hooksPath .githooks
	@echo "core.hooksPath を .githooks に設定した。"

# --- ローカル環境 ---------------------------------------------------------

# docker が無い環境で compose のターゲットを叩いたときに、何が足りないのかを示す。
.PHONY: require-docker
require-docker:
	@command -v docker >/dev/null 2>&1 \
		|| { echo "docker が見つかりません。Docker Desktop 等を入れてから実行してください。"; exit 1; }

up: require-docker ## Start the local stack, apply migrations and load the seed data
	docker compose up -d --wait
	$(MAKE) migrate
	$(MAKE) seed

migrate: ## Apply the schema migrations to the local database
	DATABASE_URL=$(LOCAL_DATABASE_URL) go run ./cmd/migrate up

seed: ## Load the seed data into the local database
	DATABASE_URL=$(LOCAL_DATABASE_URL) go run ./cmd/seed

down: require-docker ## Stop the local stack, keeping the PostgreSQL and Redis volumes
	docker compose down

downd: require-docker ## Stop the local stack and drop its data volumes
	docker compose down -v

reset: downd up ## Recreate the local stack from empty data volumes

logs: require-docker ## Follow the logs of every service
	docker compose logs -f

# --- 全体 -----------------------------------------------------------------

build: backend-build frontend-build ## Build the backend and the frontend

test: backend-test frontend-test ## Run the backend and frontend tests

lint: backend-lint frontend-lint frontend-typecheck ## Lint and type-check the backend and the frontend

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

frontend-typecheck: ## Type-check the Next.js app
	cd web && npm run typecheck

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
