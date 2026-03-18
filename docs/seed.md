# seed

Download documents into a local directory for ingestion.

## Usage

```bash
./bin/go-to-rag seed [directory]
```

Default directory: `./seeds`

| Flag         | Default                              | Description                          |
|--------------|--------------------------------------|--------------------------------------|
| `--manifest` | built-in `internal/seed/seed_data.yaml` | Path to a custom YAML manifest    |

If `--manifest` is not set, the binary uses `seed_data.yaml` compiled into it at build time via
`//go:embed` for development purposes.

## Custom manifest

```yaml
docs:
  - url: https://raw.githubusercontent.com/example/repo/main/doc.md
    name: doc.md
  - url: https://example.com/other.md
    name: other.md
```

Pass it with `./bin/go-to-rag seed --manifest my-docs.yaml ./my-docs`.

URLs must point to raw content. GitHub `blob/` and `tree/` URLs are rejected at parse time.
Use `raw.githubusercontent.com` instead.

## Default corpus

12 documents embedded in the binary (`internal/seed/seed_data.yaml`):

- Kubernetes: Pods, Operators, CRDs
- OLM (Operator Lifecycle Manager)
- OpenShift
- Kubebuilder

## Implementation

The manifest YAML is parsed into `seed.Manifest{ Docs []Doc }` where each `Doc` has a `URL`
and `Name`. The default manifest is embedded at compile time via `//go:embed seed_data.yaml`
and parsed with `gopkg.in/yaml.v3`.

Each document is fetched with `http.DefaultClient` under a 30-second per-file context timeout.
The destination file is written only after the full body is read into memory, so a failed
download leaves no partial file. Existing files are detected with `os.Stat` before the request
is made, so re-runs skip them with no network activity.

`Run` returns the number of files written. Zero means everything was already present.

## Notes

- Existing files are skipped, re-running is safe.
- Requires internet access to fetch documents.
- Does not require Ollama.