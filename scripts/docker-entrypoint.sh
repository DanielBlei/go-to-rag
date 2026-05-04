#!/bin/sh
set -e

if [ "$1" = "demo" ]; then
    shift
    PROMPT="${*:-$DEMO_PROMPT}"
    go-to-rag seed
    go-to-rag ingest --chat-host "$OLLAMA_HOST" --embed-model "$EMBED_MODEL"
    go-to-rag ask --chat-host "$OLLAMA_HOST" --chat-model "$CHAT_MODEL" --embed-model "$EMBED_MODEL" "$PROMPT"
else
    exec go-to-rag "$@"
fi
