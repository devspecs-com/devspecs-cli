---
title: Eval To CLI Integration Plan
kind: plan
status: draft
tags: [eval, cli, retrieval, integration, product]
---

# Eval To CLI Integration Plan

## Honest Current State

The first eval harness was useful, but it was too far from the live product path:

```text
filesystem fixture -> in-memory files -> eval_weighted_files_v0 -> metrics
```

The live CLI path is different:

```text
ds scan -> SQLite index -> ds find / ds resume / ds context
```

That means schema, indexing, capture, ingestion, adapter coverage, stored metadata, and source discovery can all materially affect retrieval quality. If eval does not exercise those pieces, we risk optimizing a rehearsal path while the real CLI stays weak.

Priority correction:

- Indexed eval should be the default.
- Raw filesystem eval is only a diagnostic fallback.
- Retrieval improvements should be tested against indexed artifacts before we trust them.
- The next product milestone should improve query-driven retrieval in existing CLI workflows before adding new surface area.

Related docs:

- `docs/plans/2026-05-13-retrieval-improvement-test-index.md`
- `docs/plans/2026-05-13-retrieval-quality-improvement-plan.md`
- `docs/plans/2026-05-13-language-agnostic-intent-graph-plan.md`

## Product JTBD

The core job is not:

> Export this one artifact I already know about.

The core job is:

> Given this repo and what I am trying to do, find the relevant intent, decisions, plans, constraints, and source anchors, then assemble just enough context for a human or LLM agent to act.

That points toward:

```bash
ds scan
ds find "stripe_event_id idempotency"
ds resume
ds resume "entitlement sync hardening"
```

`pack` should be treated as an internal capability: retrieve, rank, and assemble context under a token budget. It may become a public command later, but the first product bridge should reuse `ds find` and `ds resume` so the CLI does not grow before retrieval quality improves.

`ds context <id>` is still a single-artifact export. It should not become the main query-driven workflow, but removal should wait until an existing or replacement workflow clearly covers its use case.

## Integration Ladder

### Phase 0: Filesystem Lab Harness

Status: diagnostic fallback

The original harness walked fixture files directly. This remains useful only for isolating retrieval scoring from scan/index problems.

Diagnostic command:

```bash
ds eval ./fixtures/agentic-saas-fragmented --filesystem
```

Do not use filesystem-only results for product or marketing claims.

### Phase 1: Indexed Corpus Eval

Status: implemented/default

`ds eval` should scan the fixture into an isolated SQLite index and evaluate indexed artifacts by default.

Default command:

```bash
ds eval ./fixtures/agentic-saas-fragmented
```

Expected labels:

```text
corpus_source: sqlite_index
product_path: indexed_harness
```

Why this matters:

- Scan discovery affects recall.
- Adapter parsing affects artifact identity and metadata.
- Schema/storage affects what retrieval can see.
- Missing source/content ingestion becomes visible as eval misses.
- We stop tuning a path that users never hit.

Exit criteria:

- [x] `ds eval` defaults to `corpus_source: sqlite_index`.
- [x] `--filesystem` remains available as a diagnostic fallback.
- [x] Eval JSON includes `corpus_source` and `product_path`.
- [x] Timestamped result files are written for indexed runs.
- [x] Indexed eval exposes scan/index misses honestly.

### Phase 2: Shared Retrieval Core

Status: implemented

Goal:

Move retrieval logic out of `internal/evalharness` into a package usable by eval and live commands.

Package:

```text
internal/retrieval
```

Core candidate shape:

```go
type Candidate struct {
    ID       string
    Path     string
    Kind     string
    Subtype  string
    Title    string
    Status   string
    Body     string
    Source   string
    Metadata map[string]string
}
```

Adapters:

- SQLite artifacts/revisions/sources/todos/tags -> retrieval candidates
- diagnostic fixture files -> retrieval candidates

Exit criteria:

- [x] `ds eval` calls shared retrieval logic.
- [x] `internal/retrieval` exposes shared candidates, retriever interface, query baseline, and artifact reasons.
- [x] Eval output labels the shared retriever name.
- [x] Tests cover the shared retrieval package and indexed eval behavior.
- [ ] `ds find` and query-focused `ds resume` call the same retrieval logic. This belongs to Phase 3.
- [ ] Internal context assembly calls the same retrieval logic if a public `pack` command is added later.

### Phase 3: Improve Existing CLI Workflows

Status: next

Goal:

Put query-driven indexed retrieval where users already are before adding another command.

Candidate commands:

```bash
ds find "stripe_event_id idempotency"
ds resume "entitlement sync hardening"
```

Expected behavior:

- Load indexed candidates for the current repo.
- Run the shared retriever.
- Return ranked artifacts with reasons, source paths, and relevance signals.
- For query-focused `resume`, assemble a compact continuation context from selected artifacts.
- Print token counts when a context bundle is rendered.
- Keep output parseable with `--json`.
- Avoid introducing a public `pack` command until the existing workflows prove the retrieval behavior.

`ds context <id>` can remain as a precise single-artifact export during this phase.

Exit criteria:

- `ds find` uses shared indexed retrieval or has an explicit migration path to it.
- `ds resume <query>` produces query-focused continuation context from indexed artifacts.
- Output is useful to humans and agents.
- Reasons and source paths are visible.
- Support `--json`.

### Phase 4: Live CLI Regression Eval

Goal:

Make eval test the actual command path users rely on.

Possible commands:

```bash
ds eval ./fixtures/agentic-saas-fragmented --command resume-query
ds eval ./fixtures/agentic-saas-fragmented --command find
```

Behavior:

- Build fixture index in an isolated environment.
- Invoke the selected command runner.
- Parse included or ranked artifacts from JSON output.
- Compare recall, precision, sufficiency, and token reduction where context is rendered.

Expected labels:

```text
corpus_source: sqlite_index
product_path: live_cli_command
command_under_test: resume-query
```

Exit criteria:

- Eval catches regressions in scan, index conversion, retrieval, context assembly, and command output.
- Marketing claims can cite live-command eval, not just lab or indexed harness eval.

### Phase 5: Public Context-Packing UX Decision

Goal:

Decide whether context packing should remain inside `ds resume <query>` / `ds find --context`, become a new `ds pack <query>` command, or replace/deprecate `ds context <id>`.

Decision inputs:

- Live-command eval results.
- Real repo smoke tests.
- User workflow feedback.
- Whether `ds context <id>` still has a distinct job.

Possible future command, only if justified:

```text
ds pack "give agent context to implement webhook replay protection"
```

Exit criteria:

- The CLI surface area decision is explicit.
- If `ds pack` is added, it reuses the shared retrieval/core assembly path already tested through existing commands.
- If `ds context <id>` is removed or deprecated, the replacement workflow is measured and documented.

### Phase 6: Default Product Behavior

Goal:

Promote the best retriever and query workflow only after indexed and live-command evals support the claim.

Exit criteria:

- Locked benchmark fixture exists.
- Indexed eval passes thresholds.
- Live command eval passes thresholds.
- Timestamped result files are attached to release/CI evidence.
- Claims distinguish seed smoke, indexed benchmark, and live CLI benchmark.

## Marketing Maturity Labels

Current acceptable label:

```text
eval_stage: seed_smoke
corpus_source: sqlite_index
retriever: eval_weighted_files_v0
product_path: indexed_harness
```

Still not acceptable:

> DevSpecs CLI retrieves indexed agent context with 80% token reduction.

That requires `product_path: live_cli_command`.

Acceptable after indexed eval:

> DevSpecs has an indexed seed eval that measures token reduction versus retrieval quality through the scan/index pipeline.

Acceptable after live-command eval:

> DevSpecs' query-driven CLI retrieval and context assembly are measured against locked retrieval benchmarks.

## First Concrete Implementation Sequence

1. Make indexed eval the default.
2. Keep filesystem eval as `--filesystem` diagnostic only.
3. Add shared retrieval package.
4. Upgrade `ds find` to use or prepare for shared indexed retrieval with reasons.
5. Add query-focused `ds resume <query>` using shared indexed retrieval.
6. Add live-command eval mode for the existing command path.
7. Decide whether public `ds pack <query>` is needed after the existing workflows are measured.

## Auditable Success Criteria

This integration plan is successful when:

- `ds eval` defaults to an indexed fixture corpus.
- Eval JSON reports `corpus_source` and `product_path`.
- `ds find` and/or `ds resume <query>` use the shared indexed retrieval path.
- Query-focused resume can assemble context from indexed artifacts.
- Eval can run against the live command path.
- `ds context <id>` has an explicit keep/deprecate/remove decision, based on measured workflow overlap.
- Marketing claims identify whether numbers came from filesystem diagnostic, indexed harness, or live command eval.

## Open Questions

- Should query-focused context assembly live under `ds resume <query>`, `ds find --context`, a future `ds pack <query>`, or some combination?
- What exact JSON output contract should live-command eval parse?
- Should context assembly include source files not represented as artifacts, or should scan/indexing be expanded first?
- How much original source content should the index store versus reference by path at pack time?
