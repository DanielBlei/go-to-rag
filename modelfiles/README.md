# Modelfiles

Pre-tuned Ollama Modelfiles for the built-in K8s/OLM/OpenShift/Kubebuilder knowledge base. All are configured for deterministic, factual output (low temperature, controlled sampling) rather than creative generation. The system prompt is scoped to this domain.

## Models

| Model | File | Use case | VRAM                 |
|-------|------|----------|----------------------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development and fast iteration, CPU-friendly | >= 2 GB (CPU or GPU) |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, balanced quality | >= 6 GB GPU          |
| `qwen3.5:2b` | `qwen3.5-2b.Modelfile` | Fast, verbose answers with Qwen3.5 | >= 3 GB GPU          |
| `qwen3.5:4b` | `qwen3.5-4b.Modelfile` | Concise, higher-quality answers with Qwen3.5 | >= 4 GB GPU          |
| `qwen3:8b` | `qwen3-8b.Modelfile` | Production-grade with Qwen3, strongest reasoning | >= 7 GB GPU          |

The llama3.1-8b and Qwen3.5 Modelfiles set `num_gpu 99` to pin all layers on the GPU. On a machine without a discrete GPU, remove that line and the model will fall back to CPU offloading (slower).

## Parameters

Llama and Qwen3.5 use different sampling defaults, the values below reflect each model family's recommended instruct-mode settings.

| Parameter | llama3.2:1b | llama3.1:8b | qwen3:8b | qwen3.5:2b / qwen3.5:4b | Rationale |
|-----------|-------------|-------------|--------------------------|-----------|
| `temperature` | 0.1 | 0.1 | 0.1 | 0.1 | Near-deterministic output; RAG answers should be grounded, not creative |
| `num_ctx` | 8192 | 8192 | 8192 | 8192 | 8k context fits RAG chunk retrieval (top-k × chunk size) |
| `top_k` | 40 | 40 | 20 | 20 | Qwen has sharper probability distribution; 20 is the model authors' recommendation |
| `top_p` | 0.9 | 0.9 | 0.8 | 0.8 | Tighter nucleus sampling for Qwen instruct mode |
| `repeat_penalty` | 1.1 | 1.05 | 1.0 | 1.0 | Qwen doesn't benefit from repeat penalty; llama values discourage verbatim repetition |

### Context window and VRAM

All models are configured at 8192 tokens context. This fits RAG chunk retrieval without excessive VRAM. To reduce VRAM further on resource-constrained systems, lower `num_ctx` in the Modelfile (e.g., to 4096).

## Model Comparison

The table below reflects observed behavior running `make run-demo` on a GPU-equipped machine. Results will vary by hardware, but the relative tradeoffs hold:

| Model | Time | Style |
|-------|------|-------|
| `llama3.2:1b` | ~3s | Fast, minimal |
| `llama3.1:8b` | ~6s | Production-grade, balanced |
| `qwen3:8b` | ~11s | Production-grade, technical |
| `qwen3.5:2b` | ~15s | Fast, verbose |
| `qwen3.5:4b` | ~35s | Slower, concise |

> **Note:** Qwen3.5 latency may be affected by an issue with llama.cpp. Continuous testing in progress; results will be updated as improvements are observed.
>
> **Timing caveat:** All times measured with models already loaded in Ollama (no cold start). First-token latency will be higher on initial load.


## Applying changes

Review the parameters above, fine-tune the Modelfile if needed, then from the project root:

```bash
make model-create   # (re)create -- always recreates, even if the model already exists
```

Run `make help` to see all available targets.
