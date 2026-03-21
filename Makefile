BINARY     := go-to-rag
MODEL_NAME := go-to-rag:latest
MODELFILE  := modelfiles/llama3.2-1b.Modelfile

.DEFAULT_GOAL := help

.PHONY: help build test test-v test-cover cover lint lint-fix fmt tidy \
	clean run-demo run-seed run-ingest model-create model-delete

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

build: ## Build the binary
	go build -o bin/$(BINARY) .

WITH_FALLBACK ?= false
ifeq ($(WITH_FALLBACK),true)
FALLBACK_FLAG := --with-fallback
else
FALLBACK_FLAG :=
endif

run-demo: build model-delete model-create run-seed run-ingest ## Build model, seed + ingest docs, then ask a question
	./bin/$(BINARY) ask --model $(MODEL_NAME) $(FALLBACK_FLAG) \
		"How does OLM manage the lifecycle of Operators on OpenShift?"

run-seed: build ## Seed sample documents to ./seeds
	./bin/$(BINARY) seed

run-ingest: build run-seed ## Embed seeded documents into the vector store
	./bin/$(BINARY) ingest ./seeds

##@ Model

model-create: ## Create/update the custom Ollama RAG model from $(MODELFILE)
	@ollama create $(MODEL_NAME) -f $(MODELFILE)

model-delete: ## Remove the custom Ollama RAG model (ignore if not present)
	@ollama rm $(MODEL_NAME) 2>/dev/null || true

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