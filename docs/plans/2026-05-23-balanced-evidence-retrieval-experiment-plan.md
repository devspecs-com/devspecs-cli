# Balanced Evidence Retrieval Experiment Plan

Date: 2026-05-23

## Goal

Test a conservative evidence-graph reranker inside `devspecs-cli` without promoting it to default behavior.

The experiment should answer whether corroborated evidence improves real-repo retrieval precision and ranking over the current weighted retriever.

## Scope

- Add an opt-in retrieval mode for evals.
- Reuse already-indexed artifact, section, classifier, authority, and source metadata.
- Do not change discovery, classification, scanning, adapters, or default `ds find` / `ds resume-query` behavior.
- Keep broad body text and explicit reference expansion out of the first pass.

## Evidence Strategy

For candidates already admitted by the existing retriever:

- apply v0 after the normal selection/budget pipeline, so evidence reranks and annotates selected candidates rather than admitting new candidates
- boost path/title matches using repo-local rarity over the current candidate set
- boost recognized artifact roles when the query asks for that role
- boost indexed section matches as supporting file evidence
- boost anchored identifiers only when they also have path/title/section/role support
- apply small lifecycle and authority adjustments only to candidates with positive evidence
- add sibling contrast within related path groups so the strongest sibling wins
- dampen body-only candidates when a better-anchored candidate exists

## Guardrails

- Existing scoring remains the admission gate.
- Existing selection remains the v0 guardrail for the first treatment; a later treatment may test evidence-aware selection only if rerank-only improves utility.
- No repo-specific path names or labels.
- No broad noun/entity extraction.
- No evidence boost for non-intent lanes unless query intent asks for them.
- Test and code-comment artifacts keep their existing budgets.

## Auditable Success Criteria

- New eval flag selects the experimental retriever and changes the retriever name in JSON/text output.
- Unit tests cover:
  - evidence mode name
  - path/title plus role/section corroboration beating body-only noise
  - body-only hits not being promoted over anchored artifacts
- Dev-tier real50 eval completes successfully.
- Full real50 eval is attempted with the existing bounded runner.
- Compare against the current canonical aggregate on:
  - weighted mean artifact precision
  - weighted mean artifact recall
  - weighted mean must-have recall
  - weighted context sufficiency
  - must-hit@3
  - total low-precision sufficient cases

## Promotion Gate

Do not promote by default unless the full real50 run improves precision or must-hit ranking without a meaningful recall/sufficiency regression.

If metrics are mixed, keep the mode available only as an experiment and use per-case diagnostics to identify the next narrower patch.

## Results

### Initial Treatment: Evidence-Aware Selection

The first implementation applied evidence before selection. Full real50 improved rank metrics, but hurt inclusion metrics, so it should not be promoted:

- Run: `devspecs-sample-miner/_ignore/real-retrieval-real50-20260522-sparse-tests-v2/eval-runs/balanced-evidence-full-20260523/aggregate.json`
- Precision: `0.3119`
- Recall: `0.6976`
- Must-have recall: `0.7759`
- Sufficiency: `0.7586`
- Must-hit@3: `0.7845`

### Current Treatment: Rerank-Only Evidence

The adjusted implementation applies evidence after the normal selection and budget pipeline. This preserves inclusion metrics while improving ranked utility:

- Control: `devspecs-sample-miner/_ignore/real-retrieval-real50-20260522-sparse-tests-v2/eval-runs/balanced-evidence-rerank-full-control-20260523/aggregate.json`
- Treatment: `devspecs-sample-miner/_ignore/real-retrieval-real50-20260522-sparse-tests-v2/eval-runs/balanced-evidence-rerank-full-20260523/aggregate.json`
- Repos/cases: `47` repos, `116` cases, `0` failures for both runs

| Metric | Control | Treatment | Delta |
| --- | ---: | ---: | ---: |
| Precision | 0.3245 | 0.3245 | 0.0000 |
| Recall | 0.7148 | 0.7148 | 0.0000 |
| Must-have recall | 0.7931 | 0.7931 | 0.0000 |
| Sufficiency | 0.7845 | 0.7845 | 0.0000 |
| Must-hit@3 | 0.6983 | 0.8017 | +0.1034 |
| Mean first must rank | 1.7960 | 1.1307 | -0.6652 |
| Mean first useful rank | 1.6695 | 1.1221 | -0.5474 |

Conclusion: balanced evidence is currently a strong ranking/context-ordering experiment, not yet a precision improvement. Keep it opt-in until we have a separate selection-stage strategy that improves precision without recall/sufficiency loss.
