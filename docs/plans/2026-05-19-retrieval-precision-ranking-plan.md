# Retrieval Precision and Ranking Plan

Date: 2026-05-19

## Context

The indexed eval now has strong discovery coverage: the latest indexed corpus pass reached 100% discovery coverage for the current `agentic-saas-fragmented` fixture, but retrieval still spends too many tokens on broad, nearby documents. The most visible misses are ranking misses, not indexing misses:

- Broad runbooks and generic plans outrank source files when a query asks for code or identifiers.
- Generic plans and agent notes receive role boosts even when they only share broad body terms.
- OpenSpec proposal/design/tasks/spec delta files are all available, but the retriever does not distinguish which child role is useful for a given query.
- RFCs are common in mined samples, but default markdown discovery does not yet index `docs/rfcs` or `rfcs`.

Recent mined sample metadata from `intent_corpus_prod_20260518-192521` shows RFC candidates commonly using path and section shapes such as `proposals/**`, `rfcs/**`, `docs/rfcs/**`, `Summary`, `Motivation`, `Proposal`, `Drawbacks`, and `Alternatives`. We should use those patterns as general coverage, not copy raw non-redistributable sample text into fixtures.

## Goals

- Improve first-index precision while preserving discovery coverage.
- Add RFC coverage to the fixture and default markdown discovery.
- Keep ranking rules explainable as general artifact and query-intent features, not per-file exceptions.
- Add tests that exercise scoring behavior on synthetic candidates so changes do not overfit the small fixture.

## Non-Goals

- Do not train a model or add external dependencies.
- Do not copy mined corpus content into `devspecs-cli`.
- Do not add a second eval lane in this pass.
- Do not remove current OpenSpec hierarchical indexing behavior.

## Implementation Plan

1. Add sample-backed RFC fixture coverage.
   - Create a small `docs/rfcs` fixture set with synthetic RFC/proposal docs using section patterns observed in mined metadata.
   - Add at least one eval case where an RFC is the primary target and nearby ADR/OpenSpec artifacts are helpful.
   - Add one distractor RFC so ranking has to discriminate within the same artifact family.

2. Index RFC-like markdown by default.
   - Add `docs/rfcs`, `docs/rfc`, `rfcs`, and `rfc` to default markdown discovery.
   - Add nested discovery for `docs/rfcs` and `docs/rfc`.
   - Infer RFC paths as `design` artifacts, matching the existing classifier model.
   - Update tests for default config, markdown discovery, and kind inference.

3. Improve scorer precision with general features.
   - Treat `source file`, `code`, `handler`, `migration`, and identifier-heavy queries as source-intent queries.
   - Boost source files when they match identifiers; downrank broad markdown artifacts when a source-intent query has weak identifier evidence.
   - Add role-aware priors for RFCs and OpenSpec proposal/design/tasks/spec deltas.
   - Make generic plan and agent-note boosts require useful core-term evidence instead of rewarding every plan-shaped file.
   - Penalize candidates that only match broad body text and miss the query's core terms.

4. Add focused unit tests.
   - Verify source-intent queries prefer exact source files over broad runbooks.
   - Verify RFC queries retrieve RFC/proposal artifacts and exclude unrelated RFCs.
   - Verify generic plan boosts do not select a plan that lacks core query evidence.

## Acceptance Criteria

- `ds eval fixtures/agentic-saas-fragmented --json --no-save` keeps discovery coverage at 100%.
- Mean artifact precision improves over the current 36.2% baseline.
- Mean must-have recall does not regress materially from the current 81.7% baseline.
- The new RFC eval case has no expected artifact missing from the indexed corpus.
- `go test -count=1 ./...` passes.

## Implemented Result

Final local indexed eval after this pass:

- Cases: 11
- Discovery coverage: 100.0%
- Mean artifact precision: 78.5% (baseline before this pass: 36.2%)
- Mean must-have recall: 93.2% (baseline before this pass: 81.7%)
- Mean artifact recall: 77.8%
- Context sufficiency: 10/11
- Mean token reduction vs full planning corpus: 93.4%
- Expected missing from indexed corpus: 0

The remaining weak area is product/background ranking. It is now token-cheaper and keeps the must-have PRD, but still admits nearby billing PRDs/plans/ADRs and misses background ADRs. That should be the next isolated precision pass rather than being mixed into source, RFC, lifecycle, or OpenSpec tuning.

## Overfitting Guardrails

- New RFC fixture docs are synthetic derivatives of observed format patterns, not copied mined content.
- Scoring logic must cite query intent, artifact role, lifecycle signal, identifier evidence, path/title/body coverage, or source context as the reason for rank changes.
- Avoid per-file names in retrieval rules except established generic path families such as `docs/rfcs`, `docs/adr`, `.cursor`, `.claude`, and `openspec`.
- Keep fixture-specific domain expansions isolated to the existing `expandedTerms` helper until a later pass replaces them with learned or indexed aliases.
