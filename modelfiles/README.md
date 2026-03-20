# Modelfiles

Pre-tuned Ollama Modelfiles for this RAG pipeline. Both are configured for deterministic, factual output (low temperature, controlled sampling) rather than creative generation.

## Models

| Model | File | Use case | VRAM |
|-------|------|----------|------|
| `llama3.2:1b` | `llama3.2-1b.Modelfile` | Development and fast iteration, CPU-friendly, near-instant responses | >= 1 GB (CPU-capable) |
| `llama3.1:8b` | `llama3.1-8b.Modelfile` | Production-like evaluation, targets a GPU-equipped machine (desktop, laptop, or server) | >= 8 GB (Q4_K_M); **12 GB recommended** at 32k context |

The 8b Modelfile sets `num_gpu 99` to pin all layers on the GPU. On a machine without a discrete GPU, remove that line and the model will fall back to CPU offloading (slower).

## Parameters

| Parameter | Value (1b / 8b) | Rationale |
|-----------|-----------------|-----------|
| `temperature` | 0.1 / 0.1 | Near-deterministic output; RAG answers should be grounded, not creative |
| `num_ctx` | 8192 / 32768 | 32k gives the 8b room to reason over larger retrieved chunks |
| `top_k` | 20 / 40 | Restricts token candidates at each step; keeps answers on-topic |
| `top_p` | 0.9 / 0.9 | Nucleus sampling threshold |
| `repeat_penalty` | 1.1 / 1.05 | Discourages verbatim repetition of source text |

### Context window and VRAM

If VRAM is tight, lower `num_ctx` to 8192 in the Modelfile. It frees 3-4 GB with no other trade-offs for short documents.

## Applying changes

Review the parameters above, fine-tune the Modelfile if needed, then from the project root:

```bash
make model-create   # (re)create -- always recreates, even if the model already exists
```

Run `make help` to see all available targets.