# First-Index Public Eval Plan

Date: 2026-05-19

## North Star

Show that a developer can install DevSpecs locally, run an initial index, and give AI agents materially better context with far fewer tokens. The public eval should make that visible with numbers, not vibes:

- first-index discovery/index coverage
- classifier precision and recall by artifact type
- retrieval precision and recall
- context sufficiency
- token reduction and saved input tokens

## Current Baseline

Existing retrieval fixture:

- Fixture: `fixtures/agentic-saas-fragmented`
- Mean token reduction: 94.0%
- Mean retrieval precision: 85.8%
- Mean artifact recall: 83.9%
- Context sufficiency: 10/11 = 90.9%
- Discovery coverage: 100%

Mined real classifier holdout:

- Fixture: `fixtures/mined-intent-samples`
- Classifier accuracy: 105/109 = 96.3%
- ADR recall: 95.8%
- RFC recall: 95.2%
- PRD recall: 92.3%
- OpenSpec bundle recall: 100%

## Decision

Add a first-index report mode to `ds eval` instead of creating a separate one-off script.

Command shape:

```sh
ds eval ./fixtures/agentic-saas-fragmented \
  --first-index-report \
  --classifier-fixture ./fixtures/agentic-saas-fragmented \
  --classifier-fixture ./fixtures/mined-intent-samples \
  --no-save
```

The primary positional fixture remains the retrieval/token-savings fixture. Each `--classifier-fixture` runs the deterministic classifier eval and contributes classification metrics to the same report.

## Report Requirements

The text report should be suitable for a quick internal or external read:

- one compact headline section
- retrieval/token section
- classifier section per classifier fixture
- residual risks and weakest cases

The JSON report should be machine-readable for CI or marketing dashboards:

- retrieval summary
- classifier summaries and per-model precision/recall
- north-star rollup metrics
- optional estimated input cost savings when an input price is supplied

## Scope Boundaries

In scope now:

- Aggregate current retrieval and classifier evals into one first-index report.
- Report saved input tokens and optional estimated input cost savings.
- Keep this deterministic and local.
- Preserve current `ds eval` behavior when `--first-index-report` is not set.

Out of scope for this pass:

- Full retrieval cases over mined real repos.
- Public HTML dashboard.
- New classifier labels for BMAD/API spec/agent plans.
- Rewriting the eval harness corpus model.

## Success Criteria

External agents should be able to audit this without thread context:

- `ds eval --first-index-report` runs against the current synthetic retrieval fixture and mined classifier holdout.
- The report includes token reduction, saved tokens, retrieval precision/recall, sufficiency, discovery, classifier accuracy, and classifier model precision/recall.
- The normal retrieval eval and classifier eval still work unchanged.
- `go test ./...` passes.

## Next After This

1. Add real mined retrieval cases for a small curated subset.
2. Promote cursor/Claude/Codex/generic plans into explicit classifier labels or local model definitions.
3. Decide whether public eval should gate on stricter thresholds such as precision >= 90%, sufficiency >= 95%, token reduction >= 90%.
