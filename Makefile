BINARY := go-to-rag

.DEFAULT_GOAL := help

.PHONY: help build test lint lint-fix fmt tidy clean run-demo

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

build: ## Build the binary
	go build -o bin/$(BINARY) .

run-demo: build ## Build and run a demo prompt
	./bin/$(BINARY) "what is RAG?"

##@ Testing

test: ## Run tests
	go test -race -count=1 ./...

##@ Code Quality

lint: ## Run linter
	golangci-lint run ./...

lint-fix: ## Run linter with auto-fix
	golangci-lint run --fix ./...

fmt: ## Format source code
	gofmt -w .

tidy: ## Tidy go modules
	go mod tidy

##@ Cleanup

clean: ## Remove build artifacts
	rm -rf bin/