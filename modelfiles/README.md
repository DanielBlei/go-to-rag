# Modelfiles

Pre-tuned Ollama Modelfiles for the built-in K8s/OLM/OpenShift/Kubebuilder knowledge base. All are configured for deterministic, factual output (low temperature, controlled sampling) rather than creative generation. The system prompt is scoped to this domain.

## Models

| Model | File | Use case | VRAM |
|-------|------|----------|------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development and fast iteration, CPU-friendly, near-instant responses | >= 1 GB (CPU-capable) |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, targets a GPU-equipped machine (desktop, laptop, or server) | >= 8 GB (Q4_K_M); **12 GB recommended** at 32k context |
| `qwen3.5:4b` | `qwen3.5-4b.Modelfile` | Mid-range Qwen3.5, GPU recommended | >= 4 GB |
| `qwen3.5:9b` | `qwen3.5-9b.Modelfile` | Stronger reasoning with Qwen3.5, GPU recommended | >= 8 GB |

The llama3.1-8b and Qwen3.5 Modelfiles set `num_gpu 99` to pin all layers on the GPU. On a machine without a discrete GPU, remove that line and the model will fall back to CPU offloading (slower).

## Parameters

Llama and Qwen3.5 use different sampling defaults — the values below reflect each model family's recommended instruct-mode settings.

| Parameter | llama3.2:1b | llama3.1:8b | qwen3.5:4b / qwen3.5:9b | Rationale |
|-----------|-------------|-------------|--------------------------|-----------|
| `temperature` | 0.1 | 0.1 | 0.1 | Near-deterministic output; RAG answers should be grounded, not creative |
| `num_ctx` | 8192 | 32768 | 32768 | Larger context gives the model room to reason over retrieved chunks |
| `top_k` | 40 | 40 | 20 | Qwen3.5 has a sharper probability distribution; 20 is the model authors' recommendation |
| `top_p` | 0.9 | 0.9 | 0.8 | Tighter nucleus sampling for Qwen3.5 instruct mode |
| `repeat_penalty` | 1.1 | 1.05 | 1.0 | Qwen3.5 doesn't benefit from repeat penalty; llama values discourage verbatim repetition |

### Context window and VRAM

If VRAM is tight, lower `num_ctx` to 8192 in the Modelfile. It frees 3-4 GB with no other trade-offs for short documents.

## Applying changes

Review the parameters above, fine-tune the Modelfile if needed, then from the project root:

```bash
make model-create   # (re)create -- always recreates, even if the model already exists
```

Run `make help` to see all available targets.