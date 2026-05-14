---
title: Retrieval Improvement Test Index
kind: plan
status: draft
tags: [eval, retrieval, experiments, regression, benchmark]
---

# Retrieval Improvement Test Index

## Purpose

This document is the high-level index of retrieval improvements to test for DevSpecs.

Each improvement should be treated as a measurable experiment:

```text
plan -> implement narrowly -> run eval -> compare saved results -> keep / revise / reject
```

The goal is not to accumulate clever heuristics. The goal is to prove incremental movement toward the north star:

> Maximize token reduction while preserving relevant repo intent.

Related plans and OpenSpec changes:

- `docs/plans/2026-05-13-retrieval-quality-improvement-plan.md`
- `docs/plans/2026-05-13-language-agnostic-intent-graph-plan.md`
- `docs/plans/2026-05-13-eval-to-cli-integration-plan.md`
- `openspec/changes/improve-retrieval-quality/`
- `openspec/changes/language-agnostic-intent-graph/`

## Current Baseline

Seed fixture:

```text
fixtures/agentic-saas-fragmented
fixture_version: agentic-saas-fragmented-v1
eval_stage: seed_smoke
corpus_source: sqlite_index
retriever: eval_weighted_files_v0
token_counter: approx_chars_div_4
```

Current measured result:

```text
Mean token reduction vs full planning corpus: 66.2%
Mean artifact recall: 27.3%
Mean must-have recall: 26.7%
Mean artifact precision: 14.3%
Context sufficiency pass rate: 0.0%
```

Interpretation:

- Indexed eval is now exposing real scan/index coverage gaps.
- Source/context candidates are not available through the indexed corpus yet.
- Several expected OpenSpec companion files and PRD/.claude/app-plan artifacts are missing or underrepresented in scan/index output.
- The old filesystem-only result was much better, which proves the product bridge matters.
- Context sufficiency is currently the clearest product gap.

Filesystem diagnostic comparison:

```text
Mean token reduction vs full planning corpus: 80.3%
Mean artifact recall: 68.8%
Mean must-have recall: 81.7%
Mean artifact precision: 36.6%
Context sufficiency pass rate: 40.0%
```

## Experiment Protocol

For every retrieval improvement:

1. Record the current baseline result file.
2. Implement the smallest coherent change.
3. Run:

```bash
ds eval ./fixtures/agentic-saas-fragmented
```

4. Keep the timestamped JSON result written under `.devspecs/eval-runs/`.
5. Use `--filesystem` only as a diagnostic comparison when you need to separate scan/index coverage from retrieval scoring.
6. Compare before/after:
   - mean token reduction
   - overall recall
   - must-have recall
   - precision
   - sufficiency pass rate
   - worst recall case
   - largest context case
   - per-case missed artifacts
   - per-case irrelevant artifacts
   - unexpected excluded hits
7. Decide:
   - **keep**: improves target metrics without unacceptable regressions
   - **revise**: promising but causes narrow regressions or excessive context growth
   - **reject**: no measurable improvement, obvious overfit, or hurts north-star metrics

Every experiment should include the result-file paths in the PR or implementation notes.

## Global Guardrails

Do not accept an improvement if:

- Mean token reduction vs full planning corpus falls below `70%`.
- Must-have recall improves only by reclassifying too many expected artifacts as `helpful` or `background`.
- Precision improves by returning empty or near-empty context.
- Sufficiency improves by weakening success criteria.
- A fix only helps one visible case through case-specific keywords.
- Generic terms like `billing`, `customer`, `auth`, `token`, or `webhook` start dominating more cases.
- Retrieval requires an LLM, embedding service, Ollama, Anthropic, OpenAI, or network call.
- TypeScript, React, Node, or any one stack becomes a core retrieval assumption.

## Decision Template

Use this shape for each improvement PR or notes entry:

```text
Experiment ID:
Change:
Hypothesis:
Expected positive movement:
Expected risk:
Before result file:
After result file:
Metric delta:
Case wins:
Case regressions:
Decision: keep / revise / reject
Notes:
```

## Improvement Index

### CLI-001: Shared Retrieval Core

Status: complete

Hypothesis:

- A shared retrieval package will prevent divergence between eval and live CLI behavior.

Scope:

- Define indexed retrieval candidates.
- Move retriever interfaces and candidate scoring out of `internal/evalharness`.
- Keep `ds eval` behavior stable while swapping it to the shared package.
- Prepare the same package for `ds find`, query-focused `ds resume <query>`, and any later context-packing command.

Expected wins:

- No immediate metric improvement required.
- Product alignment improves because eval and CLI can share one retrieval path.

Risks:

- Mechanical refactor could accidentally change eval numbers.

Keep criteria:

- Indexed eval metrics are unchanged or differences are explained.
- Filesystem diagnostic still works.
- Retriever name and result schema remain stable.

Result:

- Added `internal/retrieval`.
- `ds eval` now calls the shared retriever package.
- Indexed eval metrics remained unchanged from the control:
  - 63.8% mean token reduction vs full planning corpus
  - 27.3% mean artifact recall
  - 26.7% mean must-have recall
  - 14.3% mean precision
  - 0.0% sufficiency pass rate

### CLI-002: Existing Command Retrieval Bridge

Status: complete

Hypothesis:

- Query-driven indexed retrieval should improve existing commands before DevSpecs adds another top-level CLI noun.

Scope:

- Upgrade `ds find` to use or prepare for shared indexed retrieval with reasons and source paths.
- Add query-focused `ds resume <query>` for continuation context.
- Use the internal context-assembly capability where a compact bundle is needed.
- Keep `ds context <id>` as a precise single-artifact export unless measured overlap justifies deprecation.
- Support parseable `--json` output for eval.

Expected wins:

- First direct product payoff from the eval/retrieval work.
- Creates a live command path for future eval without expanding the CLI surface area.

Risks:

- If indexed candidate coverage remains poor, existing commands will expose weak results.
- Overloading `resume` with query behavior could confuse users unless output clearly distinguishes dashboard mode from focused mode.

Keep criteria:

- Command is useful on the fixture and on at least one real repo smoke test.
- Output is parseable enough for future live-command eval.
- `ds context <id>` has an explicit keep/deprecate/remove decision, not an assumed removal.

Result:

- `ds find` now uses `internal/retrieval` over indexed candidates and reports source paths/reasons.
- `ds resume <query>` now renders a focused continuation context using the shared retriever.
- `ds context <id>` remains a precise single-artifact export.
- Title-aware candidate matching was added so live command retrieval does not regress obvious title searches.
- Indexed eval changed from the previous control to:
  - 66.2% mean token reduction vs full planning corpus
  - 27.3% mean artifact recall
  - 26.7% mean must-have recall
  - 14.3% mean precision
  - 0.0% sufficiency pass rate

### CLI-003: Live Command Eval For Existing Workflows

Status: next

Hypothesis:

- Marketing-relevant benchmark claims require evaluating the actual command path, not only the indexed harness.

Scope:

- Add an eval mode that invokes the selected live command runner, starting with `ds resume <query>` and/or `ds find`.
- Parse included artifacts from output.
- Report the same token, recall, precision, and sufficiency metrics.

Expected wins:

- Creates the first credible bridge from benchmark to CLI claim.

Risks:

- Command-output parsing can be brittle unless `--json` is designed for eval.

Keep criteria:

- Eval JSON labels `product_path: live_cli_command`.
- Live-command eval catches retrieval, ranking, and context-assembly regressions.

### RET-000: Baseline Control

Status: complete

Purpose:

- Establish the current indexed seed-smoke baseline before retrieval changes.

Expected movement:

- None.

Measured indexed baseline:

- Corpus source: `sqlite_index`
- Product path: `indexed_harness`
- 63.8% mean token reduction vs full planning corpus
- 27.3% mean artifact recall
- 26.7% mean must-have recall
- 14.3% mean precision
- 0.0% sufficiency pass rate

Diagnostic filesystem comparison:

- Corpus source: `filesystem_fixture`
- Product path: `lab_only`
- 80.3% mean token reduction vs full planning corpus
- 68.8% mean artifact recall
- 81.7% mean must-have recall
- 36.6% mean precision
- 40.0% sufficiency pass rate

Decision:

- Keep the indexed result as the current control for product-directed retrieval work.
- Use the filesystem result only to separate scan/index coverage problems from retriever-scoring problems.

### RET-001: Language-Neutral Identifier Normalization

Hypothesis:

- Preserving and normalizing identifier variants will improve recall for exact identifiers without broadening generic topic matches.

Scope:

- snake_case
- kebab-case
- dotted identifiers
- slash paths
- CamelCase/PascalCase splits
- singular/plural light normalization

Expected wins:

- `find-stripe-event-id`
- `authorization-details-identifier`
- future non-TypeScript source/context cases

Risks:

- Over-expansion can hurt precision if every split part becomes equally strong.

Keep criteria:

- Must-have recall improves on identifier cases.
- Precision does not materially decline.
- Artifact reasons show exact identifier or normalized variant matches.

### RET-002: Path, Slug, And Dated Filename Matching

Hypothesis:

- Treating paths, slugs, and dated filenames as first-class entities will improve recall for plan and artifact lookup.

Scope:

- filenames
- directory names
- dated plan slugs
- OpenSpec change IDs
- ADR numbers
- PRD names

Expected wins:

- `dated-pnpm-plan-slug`
- OpenSpec change queries
- source path references from trial-report-like queries

Risks:

- Generic path segments such as `docs`, `plans`, `src`, `api` can add noise.

Keep criteria:

- Slug/path cases keep or improve recall.
- Precision does not regress from generic path segment matches.

### RET-003: Query Intent Classification

Hypothesis:

- Deterministic query intent labels will let retrieval use different ranking profiles for implementation, product background, stale history, source lookup, and resume workflows.

Scope:

- `implementation_context`
- `resume_work`
- `source_identifier_lookup`
- `product_background`
- `stale_or_superseded_history`
- `architecture_decision`

Expected wins:

- webhook replay implementation-context case
- `prd-background-entitlements`
- `avoid-superseded-local-entitlements`
- `auth-token-session-boundary`

Risks:

- Misclassified short queries can hide useful artifacts.

Keep criteria:

- Sufficiency pass rate improves.
- Product-background queries include PRDs without polluting implementation-context queries.
- Stale-history queries prefer stale/superseded authority.

### RET-004: Authority And Lifecycle Scoring

Hypothesis:

- Ranking by artifact authority and lifecycle compatibility will improve precision without losing must-have recall.

Scope:

- active OpenSpec design/tasks/spec
- accepted ADRs
- superseded ADRs
- PRDs
- dated plans
- agent notes
- scratch/stale files

Expected wins:

- `avoid-superseded-local-entitlements`
- webhook replay implementation-context case
- `stripe-entitlement-not-portal`
- `auth-token-session-boundary`

Risks:

- Over-penalizing stale artifacts can hurt stale-history cases.
- Over-boosting authority can include ADRs when source files are the actual must-have.

Keep criteria:

- Precision improves.
- `expected_excluded` hits decrease.
- Stale-history recall does not regress.

### RET-005: OpenSpec Companion Bundle Edges

Hypothesis:

- When a query strongly matches an OpenSpec change, deterministic companion edges should retrieve proposal, design, tasks, and spec deltas as appropriate.

Scope:

- `proposal.md`
- `design.md`
- `tasks.md`
- `specs/**/spec.md`
- change directory slug

Expected wins:

- `resume-entitlement-sync`
- webhook replay implementation-context case
- `openspec-auth-session-design-context`

Risks:

- Bundle expansion can inflate context and lower precision if always applied.

Keep criteria:

- Must-have recall improves for OpenSpec implementation cases.
- Token reduction stays above guardrail.
- Reasons identify `openspec_companion` or equivalent edge.

### RET-006: Markdown Section Boundary Extraction

Hypothesis:

- Scoring sections before files will reduce noise from broad documents and surface task/decision/rationale/stale sections more accurately.

Scope:

- headings
- checklist items
- decision/rationale blocks
- deferred/out-of-scope sections
- risks/open questions
- success criteria
- frontmatter

Expected wins:

- stale/superseded cases
- resume cases
- implementation-context cases
- PRD-background cases

Risks:

- Generic headings can create false semantic confidence.
- Section extraction can add complexity before ranking is ready.

Keep criteria:

- Sufficiency improves.
- Artifact reasons include section role.
- Precision improves or remains stable.

### RET-007: Artifact-Type Adapters

Hypothesis:

- Purpose-built adapters for OpenSpec, ADR, PRD, plans, and agent notes will improve authority and lifecycle detection more safely than generic text scoring.

Scope:

- OpenSpec adapter
- ADR adapter
- PRD adapter
- plan adapter
- Cursor/Claude note adapter

Expected wins:

- Broad improvement across recall, must-have recall, and sufficiency.

Risks:

- Too many adapter-specific boosts can become another weight maze.

Keep criteria:

- Per-adapter reasons are auditable.
- Precision improves in noisy cases.
- No artifact type becomes globally dominant across all query intents.

### RET-008: Local Alias And Co-Occurrence Expansion

Hypothesis:

- Local aliases derived from variants and repeated co-occurrence can recover semantic-ish matches without LLMs.

Scope:

- exact identifier variants
- title/slug/frontmatter aliases
- bounded co-occurrence across independent artifacts or sections
- substring aliases with conservative thresholds

Expected wins:

- identifier cases
- OpenSpec slug cases
- shorthand terminology from real plans

Risks:

- Co-occurrence can overgeneralize and collapse precision.

Keep criteria:

- Alias reasons are visible.
- Precision does not drop.
- Aliases can be traced to local evidence.

### RET-009: Graph-Aware Ranking

Hypothesis:

- Scoring artifact/entity/section edges will outperform weighted file terms for preserving intent.

Scope:

- entity overlap
- section role match
- edge strength
- artifact authority
- lifecycle compatibility
- source/path overlap
- bundle membership

Expected wins:

- Overall recall
- must-have recall
- sufficiency
- precision in generic-query cases

Risks:

- Big-bang graph scoring can make regressions harder to attribute.

Keep criteria:

- Implement in small increments.
- Each edge/scoring factor has an eval-visible reason.
- Metric movement can be attributed to a specific factor.

### RET-010: Noise Caps And Generic-Term Saturation

Hypothesis:

- Generic term saturation and per-artifact-type caps will improve precision without sacrificing must-have recall.

Scope:

- cap repeated hits of generic terms
- downweight low-information terms
- limit plan/PRD/scratch crowding by query intent
- require stronger evidence for low-authority artifacts

Expected wins:

- `dated-pnpm-plan-slug`
- `authorization-details-identifier`
- `prd-background-entitlements`
- `auth-token-session-boundary`

Risks:

- Over-aggressive caps can hide genuinely relevant broad background.

Keep criteria:

- Precision improves.
- Must-have recall does not regress materially.
- Largest context case does not grow.

### RET-011: Generic Source/Code Entity Extraction

Hypothesis:

- Language-neutral source/code entity extraction will improve source candidate recall without TypeScript overfit.

Scope:

- paths
- identifiers
- exported-looking names where detectable
- config keys
- SQL table/column/migration identifiers
- YAML/TOML/JSON keys

Expected wins:

- `find-stripe-event-id`
- `authorization-details-identifier`
- future Go/Python/YAML/SQL source-context cases

Risks:

- Source files can crowd out planning intent if source lookup intent is not detected.

Keep criteria:

- Source lookup cases improve.
- Implementation-context cases still include design/ADR/task artifacts.
- Non-TypeScript eval case is added before claiming broad source support.

### RET-012: Optional AST Or Tree-Sitter Source Adapters

Hypothesis:

- Optional language adapters can improve source context after the graph model works, but should not be required for useful retrieval.

Scope:

- Go functions/types
- Python defs/classes
- JS/TS exports
- Rust items
- Java/Kotlin/C# classes/methods
- Terraform resources

Expected wins:

- Later source-heavy fixtures.

Risks:

- High implementation cost.
- Stack overfit.
- False sense of semantic understanding.

Keep criteria:

- Only start after RET-001 through RET-011 produce a stable graph baseline.
- Feature is optional and additive.
- Eval includes non-TypeScript coverage.

### RET-013: Section-Aware Context Packing

Hypothesis:

- Packing selected sections plus minimal artifact headers can improve precision and token reduction while preserving intent.

Scope:

- include full artifact for must-have small files
- include section excerpts for large background/noisy files
- include provenance and nearby headings
- preserve enough context for human/agent continuation

Expected wins:

- Token reduction
- precision
- sufficiency if decisive sections are retained

Risks:

- Section excerpts can omit necessary surrounding rationale.
- Current metrics are artifact-level, so section packing needs extra sufficiency checks.

Keep criteria:

- Sufficiency does not regress.
- Token reduction improves or remains strong.
- Eval reports artifact-level recall plus section-level diagnostics.

## Suggested Batch Order

### Batch A: Low-Risk Normalization

- RET-001: Language-neutral identifier normalization
- RET-002: Path, slug, and dated filename matching
- RET-010: Generic-term saturation, limited to obvious repeated-hit caps

Reason:

- These target clear measured failures with low architectural commitment.

### Batch B: Intent And Authority

- RET-003: Query intent classification
- RET-004: Authority and lifecycle scoring

Reason:

- These should directly address precision and sufficiency.

### Batch C: OpenSpec And Sections

- RET-005: OpenSpec companion bundle edges
- RET-006: Markdown section boundary extraction
- RET-007: Artifact-type adapters

Reason:

- These move DevSpecs from file scoring toward intent-structure retrieval.

### Batch D: Local Graph Semantics

- RET-008: Local alias/co-occurrence expansion
- RET-009: Graph-aware ranking

Reason:

- These are powerful but higher-risk. They should be introduced after simpler signals are measurable.

### Batch E: Source Context

- RET-011: Generic source/code entity extraction
- RET-012: Optional AST/tree-sitter adapters

Reason:

- Source context matters, but it should remain stack-neutral and should not outrank planning intent by default.

### Batch F: Context Packing

- RET-013: Section-aware context packing

Reason:

- Packing changes token counts and sufficiency interpretation. Do this after retrieval is more reliable.

## Running Log

Add entries here as experiments are run.

```text
Experiment ID: CLI-001
Date: 2026-05-14
Change: Extracted weighted file retriever, query baseline, candidates, and reasons into internal/retrieval.
Before result file: not saved for this mechanical refactor
After result file: .devspecs/eval-runs/agentic-saas-fragmented/20260514T053421Z_agentic-saas-fragmented_seed_smoke_eval_weighted_files_v0.json
Summary delta: No metric movement expected or observed; indexed eval stayed at 63.8% reduction / 27.3% recall / 26.7% must-have recall / 14.3% precision / 0.0% sufficiency.
Decision: keep
Notes: Mechanical Phase 2 bridge. Next work should wire the shared package into existing commands.

Experiment ID: CLI-002
Date: 2026-05-14
Change: Wired shared retrieval into `ds find`; added query-focused `ds resume <query>`; added title-aware candidate scoring for live command usability.
Before result file: .devspecs/eval-runs/agentic-saas-fragmented/20260514T053421Z_agentic-saas-fragmented_seed_smoke_eval_weighted_files_v0.json
After result file: .devspecs/eval-runs/agentic-saas-fragmented/20260514T054719Z_agentic-saas-fragmented_seed_smoke_eval_weighted_files_v0.json
Summary delta: token reduction improved from 63.8% to 66.2%; recall, must-have recall, precision, and sufficiency stayed flat at 27.3% / 26.7% / 14.3% / 0.0%.
Decision: keep
Notes: Product bridge now exists without adding a public `ds pack` command. Next work should add live-command eval over `ds find` and query-focused `ds resume <query>`.
```
