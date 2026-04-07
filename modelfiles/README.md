# Modelfiles

Pre-tuned Ollama Modelfiles for the built-in K8s/OLM/OpenShift/Kubebuilder knowledge base. All are configured for deterministic, factual output (low temperature, controlled sampling) rather than creative generation. The system prompt is scoped to this domain.

## Models

| Model | File | Use case | VRAM     |
|-------|------|----------|----------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development and fast iteration, CPU-friendly | >= 2 GB  |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, balanced quality | >= 6 GB  |
| `qwen3:0.6b` | `qwen3-0-6b.Modelfile` | Laptop/low-RAM development, minimal footprint | >= 3 GB  |
| `qwen3:1.7b` | `qwen3-1.7b.Modelfile` | Fast with thinking/reasoning tokens, development | >= 3 GB  |
| `qwen3:8b` | `qwen3-8b.Modelfile` | Production-grade with Qwen3, strongest reasoning | >= 7 GB  |


The llama3.1-8b and Qwen3 Modelfiles set `num_gpu 99` to pin all layers on the GPU. On a machine without a discrete GPU, remove that line and the model will fall back to CPU offloading (slower).

## Thinking Mode

Qwen3 models (`qwen3:0.6b`, `qwen3:1.7b`, `qwen3:8b`) support thinking/reasoning tokens. The CLI and gRPC server expose this via the `--think` flag:

- `--think=auto` (default): Model reasons (shows thinking tokens in gray on CLI, streams them on gRPC)
- `--think=hidden`: Model reasons internally; output is suppressed (answer-only mode)
- `--think=disabled`: Model skips reasoning entirely (fastest, no thinking tokens)

Llama models do not support thinking and safely ignore the `--think` flag.

## Parameters

Llama and Qwen3 use different sampling defaults, the values below reflect each model family's recommended instruct-mode settings.

| Parameter | llama3.2:1b | llama3.1:8b | qwen3:0.6b | qwen3:1.7b / qwen3:8b | Rationale |
|-----------|-------------|-------------|------------|------------------------|-----------|
| `temperature` | 0.1 | 0.1 | 0.1 | 0.1 | Near-deterministic output; RAG answers should be grounded, not creative |
| `num_ctx` | 8192 | 8192 | 4096 | 8192 | 0.6b uses 4k to reduce RAM footprint; others fit RAG chunk retrieval at 8k |
| `top_k` | 40 | 40 | 20 | 20 | Qwen has sharper probability distribution; 20 is the model authors' recommendation |
| `top_p` | 0.9 | 0.9 | 0.8 | 0.8 | Tighter nucleus sampling for Qwen instruct mode |
| `repeat_penalty` | 1.1 | 1.05 | 1.0 | 1.0 | Qwen doesn't benefit from repeat penalty; llama values discourage verbatim repetition |

### Context window and VRAM

All models except `qwen3:0.6b` are configured at 8192 tokens context. The 0.6b model uses 4096 to reduce RAM/VRAM pressure on memory-constrained machines — RAG chunk retrieval still fits comfortably within 4k context. To reduce VRAM further on any model, lower `num_ctx` in the Modelfile (e.g., to 4096 or 2048).

## Model Comparison

The table below reflects observed behavior running `make run-demo` on a GPU-equipped machine with `--think=auto` (default). Results will vary by hardware, but the relative tradeoffs hold:

| Model | Time | Style                       |
|-------|------|-----------------------------|
| `qwen3:0.6b` | ~2s | Fastest, Minimal            |
| `llama3.2:1b` | ~3s | Fast, minimal               |
| `qwen3:1.7b` | ~5s | Fast with reasoning         |
| `llama3.1:8b` | ~6s | Production-grade, balanced  |
| `qwen3:8b` | ~11s | Production-grade, technical |

**Thinking mode timing:** Using `--think=disabled` or `--think=hidden` can reduce latency since the model skips reasoning overhead. Qwen models with thinking typically add 2-4 seconds per response.

> **Timing caveat:** All times measured with models already loaded in Ollama (no cold start). First-token latency will be higher on initial load.

## Applying changes

Review the parameters above, fine-tune the Modelfile if needed, then from the project root:

```bash
make model-create   # (re)create -- always recreates, even if the model already exists
```

Run `make help` to see all available targets.

## Removed Models

| Model | Reason |
|-------|--------|
| `qwen3.5:2b` | Possible compatibility issue with current Ollama/llama.cpp build — high latency observed, to be re-evaluated |
| `qwen3.5:4b` | Possible compatibility issue with current Ollama/llama.cpp build — high latency observed, to be re-evaluated |
