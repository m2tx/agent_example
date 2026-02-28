BINARY  := agent
CMD     := ./cmd/server
PORT    ?= 8080
MODEL   ?= gemini-2.5-flash

.PHONY: run build fmt vet lint tidy vendor up down

run: ## Run the server (requires GEMINI_API_KEY)
	HTTP_PORT=$(PORT) MODEL=$(MODEL) go run $(CMD)

build: ## Build the binary
	go build -o $(BINARY) $(CMD)

fmt: ## Format source code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (must be installed)
	golangci-lint run ./...

tidy: ## Tidy and vendor dependencies
	go mod tidy
	go mod vendor

up: ## Start MongoDB via docker-compose
	docker compose up -d

down: ## Stop MongoDB via docker-compose
	docker compose down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
