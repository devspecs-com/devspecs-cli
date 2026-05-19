# Mined Intent Samples

This fixture contains real GitHub samples promoted from the sample miner output. It is intended as a holdout cross-check for classifier work, separate from the synthetic `agentic-saas-fragmented` fixture.

The fixture preserves repository-relative paths under `samples/repos/<owner>__<repo>/...` so classifier path evidence remains realistic. Miner metadata frontmatter is removed from document samples; provenance is recorded in `manifest.yaml` and each classifier case.

## Source

- Miner output: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-sample-miner/intent_corpus_prod_20260518-192521`
- Promotion command: `go run ./scripts/promote_mined_classifier_samples.go`

## Counts

- `adr`: selected 24/24, classifier cases 24
- `api_spec`: selected 4/4, classifier cases 0
- `bmad`: selected 17/17, classifier cases 0
- `bmad_like_story`: selected 16/16, classifier cases 0
- `openspec_bundle`: selected 30/732, classifier cases 30
- `prd`: selected 13/13, classifier cases 13
- `rfc`: selected 42/42, classifier cases 42

## Eval

Run:

```sh
go run ./cmd/ds eval ./fixtures/mined-intent-samples --classifier --no-save
```

BMAD, BMAD-like story, and API spec samples are included in the manifest but are not classifier assertions yet because the deterministic classifier does not currently expose those as first-class model labels.
