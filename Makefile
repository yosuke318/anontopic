.PHONY: help build run test lint fmt web-install web-build web-lint web-format check

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

check: build test lint web-lint web-build ## Run every check used in CI
