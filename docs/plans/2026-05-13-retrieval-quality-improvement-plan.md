---
title: Retrieval Quality Improvement Plan
kind: plan
status: draft
tags: [eval, retrieval, context, quality]
---

# Retrieval Quality Improvement Plan

## Context

The `agentic-saas-fragmented-v1` seed eval now gives us an honest indexed starting point:

- Eval stage: `seed_smoke`
- Corpus source: `sqlite_index`
- Product path: `indexed_harness`
- Retriever: `eval_weighted_files_v0`
- Token counter: `approx_chars_div_4`
- Planning corpus: 26 indexed files / ~14,318 tokens
- Source/context candidates: 0 indexed files
- Mean token reduction vs full planning corpus: ~63.8%
- Mean artifact recall: ~27.3%
- Mean must-have recall: ~26.7%
- Mean artifact precision: ~14.3%
- Context sufficiency pass rate: 0.0%

Diagnostic filesystem-only eval still scores higher, but that path is no longer the product-adjacent baseline:

- Mean token reduction vs full planning corpus: ~80.3%
- Mean artifact recall: ~68.8%
- Mean must-have recall: ~81.7%
- Mean artifact precision: ~36.6%
- Context sufficiency pass rate: 40.0%

Interpretation: the indexed path exposes scan/index/schema/adapter coverage as a core retrieval-quality problem. The next product goal is not just better ranking; it is better indexed candidate coverage plus shared retrieval logic that can power a real query-driven CLI workflow.

This plan is based on the seed eval plus the May 12 trial report from `pdpp` and `unity-surfaces`.

Related OpenSpec change: `openspec/changes/improve-retrieval-quality/`.

## North Star

Maximize token reduction subject to acceptable intent retrieval quality.

Target direction for v0:

- >=70% token reduction vs full planning corpus
- >=85% artifact recall
- >=60% artifact precision
- >=90% must-have artifact recall
- >=70% context sufficiency pass rate
- Clear missed and noisy artifacts for every case

Do not optimize token reduction alone. Empty or overly tiny context is not useful.

## Current Implementation Priority

Implement the eval-to-CLI bridge before additional heuristic tuning.

Priority order:

1. Keep indexed eval as the default measurement path.
2. Extract shared retrieval candidate/retriever logic out of `internal/evalharness`.
3. Wire shared retrieval into existing CLI workflows first, starting with `ds find` and query-focused `ds resume <query>`.
4. Add live-command eval for the existing command path.
5. Decide whether a public `ds pack <query>` command is still needed after the existing workflows are measured.
6. Then iterate on retrieval improvements from the test index.

Reason: schema, scan discovery, adapters, indexed metadata, and source/content availability materially affect eval results. Optimizing a filesystem-only retriever would hide the product gap.

`pack` remains a useful internal concept: retrieve, rank, and assemble a compact context bundle under a token budget. It should not be treated as the next public CLI surface until the current commands prove the retrieval behavior.

## Auditable Success Criteria

These criteria should be auditable from repository state and deterministic command output. A reviewer should not need to rely on subjective judgment or LLM grading.

### Eval Schema and Reporting

- `cases.yaml` supports importance-weighted expected artifacts with `must`, `helpful`, and `background`.
- Legacy string-form `expected_relevant` entries still work.
- Eval output reports:
  - token reduction vs `full_planning_corpus`
  - token reduction vs `query_file_baseline`
  - overall artifact recall
  - must-have artifact recall
  - helpful artifact recall
  - background artifact recall
  - artifact precision
  - context sufficiency pass rate
  - missed expected artifacts
  - unexpected forbidden artifacts
- JSON output contains the same metrics as text output.

Audit command:

```bash
ds eval ./fixtures/agentic-saas-fragmented --json
```

Every eval run should also write a timestamped JSON artifact under `.devspecs/eval-runs/` by default. Use `--results-dir` to redirect saved artifacts for CI, release evidence, or benchmark comparison logs.

### Retrieval Quality Targets

For `fixtures/agentic-saas-fragmented` at `eval_stage: seed_smoke`, retrieval is considered improved when the eval reports:

- Mean token reduction vs full planning corpus: `>= 70%`
- Overall artifact recall: `>= 80%`
- Must-have artifact recall: `>= 85%`
- Artifact precision: `>= 50%`
- Context sufficiency pass rate: `>= 70%`

These are seed-stage targets, not marketing claims. A future `locked_benchmark` fixture may use stricter thresholds.

### Regression Guardrails

The improvement is not acceptable if any of the following occur:

- Mean token reduction falls below `70%`.
- Must-have recall improves only by marking too many artifacts as `helpful` or `background`.
- Precision improves by returning empty or near-empty context.
- Known must-not-include artifacts from `expected_excluded` appear without being reported.
- Eval output omits missed artifacts.
- Eval requires a network call, model call, Ollama, OpenAI, Anthropic, or external service.

### Trial-Report Coverage

At least one deterministic case must cover each trial-report gap:

- Identifier-shaped query, such as `authorization_details` or `stripe_event_id`.
- Dated/slug-style plan filename, such as `260219-pnpm-migration.md`.
- OpenSpec implementation context requiring `design.md`.
- OpenSpec context requiring spec deltas.
- Stale or superseded local entitlement context.
- Auth token/session ambiguity that should not retrieve billing webhook docs as primary context.
- Product-background query where PRD inclusion is expected.
- Implementation-context query where PRDs are expected to be excluded.

Audit evidence:

- The case IDs and expected artifacts are present in `fixtures/agentic-saas-fragmented/cases.yaml`.
- The eval output lists per-case missed and irrelevant artifacts.

### Explainability

For each retrieved artifact, JSON output should expose at least one deterministic reason, such as:

- query term match
- identifier match
- path/title/body match
- OpenSpec bundle inclusion
- authority or lifecycle signal
- query intent signal

Audit expectation:

- A reviewer can inspect why a noisy artifact was included without reading the retriever code.

### OpenSpec Bundle Behavior

For implementation-context queries that match an active OpenSpec change, the retriever should consider:

- `proposal.md`
- `design.md`
- `tasks.md`
- `specs/**/spec.md`

Audit expectation:

- The eval has at least one case where missing `design.md` or a spec delta lowers must-have recall or sufficiency.
- The retrieved artifact list shows whether those OpenSpec companion files were included.

### Evidence to Capture Before and After Retrieval Changes

For every retrieval improvement PR, include:

- `ds eval ./fixtures/agentic-saas-fragmented` text summary.
- The timestamped JSON result file written by `ds eval`, or `ds eval ./fixtures/agentic-saas-fragmented --json` output saved or attached in CI logs.
- A short before/after table:
  - mean token reduction
  - overall recall
  - must-have recall
  - precision
  - sufficiency pass rate
  - worst recall case
  - largest context case

## Trial-Report Retrieval Gaps

The trial report highlights concrete product gaps that should guide retrieval work:

- Identifier-shaped terms: `client_display`, `pnpm`, `authorization_details`, snake_case, kebab-case, dotted paths.
- Dated/slug-style plan filenames: e.g. `260219-pnpm-migration.md`.
- OpenSpec context completeness: include `design.md` and spec deltas, not only proposal/tasks.
- Lifecycle semantics: distinguish active, ready-to-archive, stale, superseded, and unknown-status artifacts.
- Result explainability: body snippets, paths, and line-level matches are needed for disambiguation.
- Slug and filename identity: users should be able to reference stable human slugs, not only short hashes.
- Missed source locations: scan/discovery should surface likely plan dirs outside defaults.

## Completed Eval Additions

The first eval hardening pass is complete:

- Importance-weighted relevance: `must`, `helpful`, `background`
- Overall, must-have, helpful, and background recall
- Context sufficiency criteria
- Per-case sufficiency pass/fail
- Aggregate sufficiency pass rate
- Timestamped JSON result files
- Indexed SQLite corpus as the default eval path
- Filesystem eval as a diagnostic fallback

This keeps the eval deterministic while getting closer to the product question: "Can the indexed CLI path retrieve enough relevant intent for an agent or human to continue?"

## Retrieval Improvement Workstreams

### 1. Identifier-Aware Search

Goal: searches for protocol and code identifiers should find matching bodies and paths.

Tasks:

- Preserve snake_case, kebab-case, dotted, slash, and package-manager terms during indexing/search.
- Search source path, artifact slug, title, body, and extracted task text.
- Add tests for `authorization_details`, `stripe_event_id`, `client_display`, `pnpm`, and dated filenames.
- Avoid treating common separators as term deletion.

Expected eval impact:

- Improve source-file recall for identifier cases.
- Improve dated plan slug recall.
- Improve must-have recall for source-hit cases.

### 2. OpenSpec Bundle Retrieval

Goal: an OpenSpec change should retrieve the relevant proposal, design, tasks, and spec deltas as a coherent bundle when the query is about implementation context.

Tasks:

- Model OpenSpec change directories as grouped artifacts or linked artifact bundles.
- Include `design.md` and `specs/**/spec.md` in context generation when appropriate.
- Preserve ability to choose only a subset for narrow queries.
- Make bundle inclusion explicit in output so users understand why related files appeared.

Expected eval impact:

- Improve recall for the webhook replay implementation-context case.
- Improve recall for auth-session design/spec-delta cases.
- Reduce misses where `design.md` is decisive.

### 3. Authority and Lifecycle Signals

Goal: prefer authoritative, current intent artifacts over stale scratch plans and broad support residue.

Tasks:

- Infer authority from path and format:
  - ADR accepted/superseded
  - OpenSpec active/archive state
  - PRD background
  - Cursor/Claude notes
  - scratch/stale notes
- Treat status fields and frontmatter consistently.
- Penalize stale scratch unless the query asks for stale/superseded/history.
- Distinguish "ready-to-archive" OpenSpec changes from active in-progress work.

Expected eval impact:

- Improve precision for stale local entitlement caching.
- Reduce inclusion of broad runbooks and scratch notes.
- Keep superseded ADRs discoverable when explicitly requested.

### 4. Query Intent Classification

Goal: retrieve different artifact types depending on whether the user asks for implementation context, product background, historical rationale, source files, or resume state.

Tasks:

- Add deterministic intent buckets:
  - implementation context
  - product/background
  - source/code identifier
  - stale/history/rationale
  - resume/continue
- Use intent buckets to adjust artifact-type preference, not case-specific file weights.
- Keep rules inspectable and deterministic for v0.

Expected eval impact:

- Improve PRD background cases.
- Avoid PRDs for implementation-only cases.
- Improve precision for auth token/session versus billing webhook ambiguity.

### 5. Result Explainability

Goal: users and tests should see why an artifact was selected.

Tasks:

- Include score reason fragments in eval JSON:
  - matched query terms
  - path match
  - title/body match
  - authority/status hint
  - bundle inclusion
- Add optional human output for score reasons.
- Use reasons to debug retrieval changes without LLM judging.

Expected eval impact:

- Easier regression analysis.
- Better internal understanding before marketing claims.

## Work Order

1. Keep indexed eval as the default control.
2. Extract shared retrieval candidate/retriever logic out of `internal/evalharness`.
3. Upgrade `ds find` to use or prepare for shared indexed retrieval with reasons.
4. Add query-focused `ds resume <query>` for continuation context.
5. Add live-command eval for the existing command path.
6. Decide whether `ds pack <query>` is needed as a public command after results and workflow feedback.
7. Implement identifier-aware matching.
8. Implement OpenSpec bundle retrieval.
9. Add lifecycle/authority scoring.
10. Add query intent classification.
11. Re-run seed eval and compare:
   - token reduction
   - overall recall
   - must-have recall
   - precision
   - sufficiency pass rate

## Guardrails

- Do not tune weights to make visible cases perfect.
- Do not add LLM judging yet.
- Do not claim coding success.
- Preserve deterministic local evaluation through the indexed path, with filesystem mode only as a diagnostic comparison.
- Keep `eval_stage: seed_smoke` until cases and distractors are locked.
- Prefer improvements justified by trial-report failures over fixture-specific hacks.

## Expected Outcome

Near-term success is not 100% precision/recall. It is a believable curve showing retrieval quality improving while token reduction remains substantial.

The next credible milestone would look like:

- 70-85% token reduction vs full planning corpus
- 80-90% overall recall
- 85-95% must-have recall
- 50-70% precision
- deterministic sufficiency pass rate reported
- missed and noisy artifacts still visible
