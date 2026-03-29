#!/bin/sh
set -e

if [ "$1" = "demo" ]; then
    shift
    PROMPT="${*:-$DEMO_PROMPT}"
    go-to-rag seed
    go-to-rag ingest --host "$OLLAMA_HOST" --embed-model "$EMBED_MODEL"
    go-to-rag ask --host "$OLLAMA_HOST" --model "$CHAT_MODEL" --embed-model "$EMBED_MODEL" "$PROMPT"
else
    exec go-to-rag "$@"
fi
