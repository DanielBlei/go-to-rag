# Corpus Snapshot

The corpus folder holds a frozen subset of the project's seed documents used by the
retrieval-evaluation harness. Treat it as test fixtures: pin the bytes, do not
re-pull at eval time.

## Snapshot date

2026-04-25

## Source URLs

| File                          | URL |
|-------------------------------|-----|
| `kubernetes_pods.md`          | https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/workloads/pods/_index.md |
| `kubernetes_operators.md`     | https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/extend-kubernetes/operator.md |
| `kubernetes_crds.md`          | https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions.md |
| `olm_architecture.md`         | https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/master/doc/design/architecture.md |
| `openshift_routes.md`         | https://raw.githubusercontent.com/openshift/enhancements/master/enhancements/ingress/custom-route-configuration.md |
| `kubebuilder_introduction.md` | https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/introduction.md |

## Frozen corpus warning

These files MUST NOT be regenerated as part of running `go-to-rag eval`. The
golden queries in `../golden.v1.json` are pinned against this exact set of
bytes. Re-pulling upstream may rotate phrasing and silently change retrieval
scores. If the corpus needs to be refreshed, bump it as a deliberate change
together with a fresh review of every golden query.

## Pre-processing applied at snapshot time

Hugo front-matter (`---\n...\n---\n` blocks) has been stripped from the four
files that carried it (`kubernetes_pods.md`, `kubernetes_operators.md`,
`kubernetes_crds.md`, `openshift_routes.md`). Front-matter is YAML metadata
intended for the upstream static-site generator and contributes only noise to
embeddings (titles repeated in the body, weights, alias lists). Production
ingest receives the same upstream files unstripped, so eval underestimates
production noise rather than overestimating it.

The remaining content still contains some Hugo shortcodes such as
`{{% heading "prerequisites" %}}` which were left in place: stripping them
risks corrupting list structure, and they appear consistently across the
corpus so they neither help nor hurt cross-document retrieval.

## Known limitations

**Sample size.** Twenty queries over six documents is the bare minimum to
distinguish "the embedding model works" from "everything zero". It is not
enough to detect small regressions in retrieval quality: a single query
changing outcome moves Hit@K by 5 percentage points. Treat metric deltas
under ~5 points as noise. To resolve a real regression, re-run with seed
variance (run twice and compare per-query) or expand the dataset.

**Topical clustering.** All six documents come from a tightly related slice
of the cloud-native ecosystem (Kubernetes core, CRDs, operators, OLM,
OpenShift, Kubebuilder). Embedding spaces collapse semantically adjacent
documents, so multi-doc queries that name two of these will often retrieve
a third unrelated doc with surprisingly high cosine similarity. This is a
property of the corpus, not a bug in retrieval. Adding documents from
unrelated domains would change the absolute metric levels but is out of
scope for Phase 1.
