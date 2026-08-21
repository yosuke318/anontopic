.PHONY: help build run test lint fmt web-install web-build web-lint web-format check load-test load-report

# 負荷テスト（Vegeta）の既定値。make load-test LOAD_RATE=200 のように上書きできる。
LOAD_RATE ?= 50
LOAD_DURATION ?= 30s
LOAD_TARGETS ?= test/load/targets.txt
LOAD_RESULT ?= test/load/results/latest.bin

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

build: ## Build all Go packages
	go build ./...

run: ## Run the API server locally
	go run ./cmd/server

test: ## Run Go tests
	go test ./...

lint: ## Run golangci-lint over the Go code
	golangci-lint run ./...

fmt: ## Format Go code
	gofmt -w .

web-install: ## Install web dependencies
	cd web && npm install

web-build: ## Build the Next.js app
	cd web && npm run build

web-lint: ## Lint the Next.js app
	cd web && npm run lint

web-format: ## Format the Next.js app with Prettier
	cd web && npm run format

load-test: ## Run a Vegeta load test against a locally running server
	@mkdir -p $(dir $(LOAD_RESULT))
	go tool vegeta attack -targets=$(LOAD_TARGETS) -rate=$(LOAD_RATE) -duration=$(LOAD_DURATION) -output=$(LOAD_RESULT)
	go tool vegeta report $(LOAD_RESULT)

load-report: ## Show the report and latency histogram of the last load test
	go tool vegeta report $(LOAD_RESULT)
	go tool vegeta report -type='hist[0,10ms,50ms,100ms,500ms,1s]' $(LOAD_RESULT)

check: build test lint web-lint web-build ## Run every check used in CI
