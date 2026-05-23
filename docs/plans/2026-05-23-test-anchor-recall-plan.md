# Test Anchor Recall And Miss Diagnostics Plan

Date: 2026-05-23

## Context

The real50 north-star eval is inside the desired compression band, but still below the trust target:

- Target: 85%+ sufficient packs, 90%+ must-have recall, 90-97% context reduction.
- Current full real50 control is roughly 78% sufficient, 79% must-have recall, and 96-97% query-baseline reduction.

The remaining gap appears recall-specific. In particular, test/source artifacts exist for several missed must-hit cases, but retrieval under-anchors exact test names such as `TestPutAndGetExposedTool` because identifier query handling only treats `_`, `-`, and `.` separated terms as identifiers. Test-case artifacts already store `test_name`, `parent_title`, symbols, assertion terms, source path, and line range.

## Hypotheses

1. Exact test-name/title anchoring will recover a meaningful fraction of missed must-have test/source hits without increasing broad markdown noise.
2. A grouped miss report by miss phase, role, and anchor availability will keep future optimization measurable instead of anecdotal.
3. Narrow protocol/template/product discovery lanes are likely the next recall patch, but should follow the test-anchor patch so we can isolate impact.

## Scope

### Implement Now

- Add camel/Pascal identifier normalization for query terms and candidate metadata/title text.
- Treat exact or near-exact `test_name` / test title / parent title matches as strong anchors for `test_case` candidates.
- Downweight generic test-behavior query words (`test`, `tests`, `cover`, `covers`, `behavior`, `what`, etc.) so they do not dominate scoring.
- Add retrieval tests for camel/Pascal/snake test-name queries.
- Extend eval diagnostics with grouped miss classes where possible:
  - missing from corpus
  - missed after discovery
  - likely test/source anchor miss
  - lane/role bucket
- Add narrow discovery support for high-signal protocol/template/product/requirement files that showed up as corpus misses:
  - `.claude/skills/**/SKILL.md`
  - `.codex/skills/**/SKILL.md`
  - `agents/**/*.agent.md`
  - `docs/product-specs/**`
  - `docs/requirements/**`
  - `REQ_*.md` / `REQ-*.md`
  - `PROPOSAL_TEMPLATE.md`
  - `MAINTAINERS.md`
  - `GOVERNANCE.md`

### Defer

- Concept-neighborhood ranking.
- Embeddings, LLM ingestion, and full semantic graphs.

## Auditable Success Criteria

- Unit tests prove `TestPutAndGetExposedTool`, `test_put_and_get_exposed_tool`, and natural-language queries like “what tests cover put and get exposed tool behavior” retrieve the relevant test artifact ahead of unrelated tests.
- The retrieval implementation records a clear evidence reason when a candidate is boosted by exact test anchor metadata.
- The real50 eval completes with 0 repo failures.
- Must-have recall improves or stays flat; if it regresses, the patch is not promoted.
- Precision does not regress by more than 1 percentage point on real50.
- The aggregate report exposes enough miss grouping to distinguish corpus misses from retrieval misses.

## Measurement

Run the current full real50 control before and after the patch, using the same manifest and flags. Compare:

- mean precision
- mean recall
- mean must-have recall
- context sufficiency
- query-baseline token reduction
- missed-after-discovery count
- missing-from-corpus count

If exact test anchors recover at least 4 must-have misses without material precision loss, proceed to narrow discovery-lane expansion next.
