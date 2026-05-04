PROJECT    := go-to-rag
MODELFILE  := modelfiles/qwen3-0.6b.Modelfile
DEMO_PROMPT ?= How does OLM manage the lifecycle of Operators on OpenShift/Kubernetes clusters?

# Inference backend
INFERENCE   ?= ollama
CHAT_HOST   ?= http://localhost:11434
CHAT_MODEL  ?= $(PROJECT):latest
EMBED_MODEL ?= mxbai-embed-large:latest
EMBED_HOST  ?=
API_KEY     ?=

# Container
CONTAINER_TOOL ?= $(shell (podman ps >/dev/null 2>&1 && echo podman) || (docker ps >/dev/null 2>&1 && echo docker))
OLLAMA_HOST    ?= http://localhost:11434
IMG            ?= $(PROJECT):latest

INFERENCE_FLAGS = --inference $(INFERENCE) --chat-host $(CHAT_HOST) \
  --chat-model $(CHAT_MODEL) --embed-model $(EMBED_MODEL) \
  $(if $(EMBED_HOST),--embed-host $(EMBED_HOST)) \
  $(if $(API_KEY),--api-key $(API_KEY))

.DEFAULT_GOAL := help

.PHONY: help build test test-cover cover lint lint-fix fmt tidy clean \
  run-demo run-seed run-ingest model-create model-delete eval \
  docker-build docker-demo docker-clean proto

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

build: ## Build the binary
	go build -o bin/$(PROJECT) .

run-demo: build model-delete model-create run-seed run-ingest ## Seed, ingest, then ask a demo question
	./bin/$(PROJECT) $(INFERENCE_FLAGS) \
	  $(if $(filter true,$(WITH_FALLBACK)),--with-fallback) \
	  ask "$(DEMO_PROMPT)"

run-seed: build ## Download sample documents to ./seeds
	./bin/$(PROJECT) seed

run-ingest: build run-seed ## Embed seeded documents into the vector store
	./bin/$(PROJECT) $(INFERENCE_FLAGS) ingest ./seeds

eval: build ## Run retrieval eval suite and print a text report
	./bin/$(PROJECT) $(INFERENCE_FLAGS) eval --format text

model-create: ## Create the custom Ollama RAG model from $(MODELFILE)
	@ollama create $(CHAT_MODEL) -f $(MODELFILE)

model-delete: ## Remove the custom Ollama RAG model (no-op if absent)
	@ollama rm $(CHAT_MODEL) 2>/dev/null || true

##@ Testing

test: ## Run tests
	go test -race -count=1 ./...

test-cover: ## Run tests and write coverage profile to coverage.out
	go test -race -count=1 -coverprofile=coverage.out ./...

cover: test-cover ## Open coverage report in browser
	go tool cover -html=coverage.out

##@ Code Quality

proto: ## Regenerate Go stubs from protobuf definitions
	buf generate

lint: ## Run linters
	golangci-lint run ./...
	@buf lint && echo "proto: ok"

lint-fix: ## Run linter with auto-fix
	golangci-lint run --fix ./...

fmt: ## Format source code
	gofmt -w .

tidy: ## Tidy go modules
	go mod tidy

##@ Docker

docker-build: ## Build the container image
	$(CONTAINER_TOOL) build -t $(IMG) .

docker-demo: docker-build model-delete model-create ## Run the full demo pipeline in a container (Ollama only)
	$(CONTAINER_TOOL) run --rm --network host \
	  -e OLLAMA_HOST=$(OLLAMA_HOST) \
	  -e CHAT_MODEL=$(CHAT_MODEL) \
	  -e EMBED_MODEL=$(EMBED_MODEL) \
	  -e DEMO_PROMPT="$(DEMO_PROMPT)" \
	  $(IMG)

docker-clean: ## Remove the container image
	$(CONTAINER_TOOL) rmi $(IMG) 2>/dev/null || true

##@ Cleanup

clean: ## Remove build artifacts
	rm -rf bin/