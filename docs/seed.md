# seed

Download documents into a local directory for ingestion.

```bash
./bin/go-to-rag seed [directory]
```

Default directory: `./seeds`

| Flag         | Default                                 | Description                      |
|--------------|-----------------------------------------|----------------------------------|
| `--manifest` | built-in `internal/seed/seed_data.yaml` | Path to a custom YAML manifest   |

## Workflow

1. **Load manifest**: parses the YAML manifest (built-in or `--manifest`). The default manifest is compiled into the binary at build time via `//go:embed`, so `seed` works offline once built.
2. **Validate URLs**: GitHub `blob/` and `tree/` URLs are rejected before any network call ,  they return HTML, not raw content. Use `raw.githubusercontent.com` instead.
3. **Download**: each document is fetched under a 30-second per-file timeout. The full body is read into memory before writing to disk ,  a failed download leaves no partial file.
4. **Skip existing**: files are checked with `os.Stat` before the request is made. Already-present files are skipped with no network activity, making re-runs safe.

`Run` returns the number of files written. Zero means everything was already present.

## Custom manifest

```yaml
docs:
  - url: https://raw.githubusercontent.com/example/repo/main/doc.md
    name: doc.md
  - url: https://example.com/other.md
    name: other.md
```

Pass it with `./bin/go-to-rag seed --manifest my-docs.yaml ./my-docs`.

## Default corpus

12 documents embedded in the binary (`internal/seed/seed_data.yaml`):

- Kubernetes: Pods, Operators, CRDs
- OLM (Operator Lifecycle Manager)
- OpenShift
- Kubebuilder

## Notes

- Does not require Ollama.
- Requires internet access to fetch documents.
- Re-running is safe ,  existing files are never overwritten.
