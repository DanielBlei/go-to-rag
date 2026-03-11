# go-to-rag

> **Status: under active development — experimental**

A local RAG (Retrieval-Augmented Generation) engine written in Go.

## What it does

- Ingests documents and indexes them using local embeddings via [Ollama](https://ollama.com)
- Retrieves relevant chunks using vector similarity search
- Answers questions using a local LLM (Ollama) — no cloud required
- Designed to be extended: an [MCP](https://modelcontextprotocol.io) interface is planned, allowing any external LLM (Claude, OpenAI, etc.) to use the retrieval engine as a tool while keeping embeddings local

## Goals

- Experiment with RAG patterns in Go
- Keep the stack simple and self-hostable (no managed services, no API keys needed to run embeddings)
- Build a clean [Model Context Protocol](https://modelcontextprotocol.io) interface that lets external LLMs use retrieval as a pluggable tool
- Compose local fast retrieval + smart reasoning (local Ollama or remote Claude/OpenAI)

## Stack

| Component | Choice | Why                                                                                                 |
|-----------|--------|-----------------------------------------------------------------------------------------------------|
| Language | Go | Fast, compiled, good ecosystem for systems/tools                                                    |
| Embeddings | Ollama (`nomic-embed-text`) | Local, fast, no API keys, good quality for retrieval                                                |
| Reasoning | Local (Ollama) or remote (Claude/OpenAI via MCP) | Choose based on needs: local keeps everything self-contained, and remote leverages better reasoning |
| Vector store | In-memory (MVP) / Database (planned) | MVP: simplicity + speed. Database: persistence at scale                                             |
| Interface | MCP (Model Context Protocol) | Standard protocol for composing LLM tools, it works with Claude, OpenAI, etc.                       |

## Contributing

This is a personal learning project. Feedback, issues, and ideas are welcome.

## License

[Apache 2.0](LICENSE)