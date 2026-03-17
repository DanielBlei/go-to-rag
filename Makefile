BINARY     := go-to-rag
MODEL_NAME := go-to-rag
MODELFILE  := modelfiles/llama3.2-1b.Modelfile

.DEFAULT_GOAL := help

.PHONY: help build test test-v test-cover cover lint lint-fix fmt tidy clean run-demo run-seed model-create model-delete

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

build: ## Build the binary
	go build -o bin/$(BINARY) .

run-demo: build model-create ## Build and run a demo prompt using the custom RAG model
	./bin/$(BINARY) --model $(MODEL_NAME) ask "what is AI RAG Pipelines?"

run-seed: build ## Seed sample documents to ./seeds
	./bin/$(BINARY) seed

##@ Model

model-create: ## Create the custom Ollama RAG model from $(MODELFILE) (skips if already present)
	@ollama list | grep -q "^$(MODEL_NAME)" || ollama create $(MODEL_NAME) -f $(MODELFILE)

model-delete: ## Remove the custom Ollama RAG model
	ollama rm $(MODEL_NAME)

##@ Testing

test: ## Run tests
	go test -race -count=1 ./...

test-v: ## Run tests with verbose output
	go test -race -count=1 -v ./...

test-cover: ## Run tests and write coverage profile
	go test -race -count=1 -coverprofile=coverage.out ./...

cover: test-cover ## Run tests and open coverage report in browser
	go tool cover -html=coverage.out

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