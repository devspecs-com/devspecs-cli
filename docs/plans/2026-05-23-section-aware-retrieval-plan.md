# Section-Aware Retrieval Plan

Date: 2026-05-23

## Goal

Promote sections from a post-retrieval packing detail into first-class indexed retrieval evidence.

Current behavior already supports section packing: after a file/artifact is selected, the context pack can include selected sections rather than the full file. The missing capability is section-aware indexing and retrieval: a large artifact should be selected because a specific indexed section matches the query, and the retrieval report should explain that section hit.

## Why This Is Highest Priority

File-level retrieval creates two precision problems:

- Broad planning files match many generic terms and enter context even when only a small section is relevant.
- Precision metrics treat the full artifact as included, while the agent actually needs a focused subsection.

Section-aware retrieval should improve:

- artifact precision, by letting a focused section carry the relevance signal
- context sufficiency, by selecting the exact requirement/design/rationale subsection
- token reduction, by avoiding full-file inclusion for long docs
- auditability, by showing heading path and line range for why an artifact was selected

## Non-Goals

- Do not build a full semantic document graph in this pass.
- Do not require manual section labels before measuring file-level impact.
- Do not introduce embeddings or LLM reranking in v0.
- Do not touch scanner/indexing speed work owned by the parallel agent.
- Do not make sections replace parent artifact identity; file/artifact recall remains the primary guardrail.

## Status Quo

Already present or partly present:

- file/artifact candidates
- OpenSpec bundle/file relationships
- post-retrieval section packing metadata such as packed section count
- file-level eval labels and retrieval metrics
- lane metrics and agent metrics
- eval phase cache/budget support

Missing:

- durable section candidate representation before retrieval
- persisted section rows in the index DB
- section-level FTS/queryability
- section scoring before parent artifact selection
- section hit explanations in `artifact_reasons`
- section-aware precision diagnostics
- upward section-to-file and section-to-bundle evidence that is visible before packing
- section-level eval diagnostics beyond post-pack counts
- lightweight eval comparison: file-level retrieval vs section-aware retrieval

## Gaps To Close In This Pass

Close these gaps as part of the section-aware retrieval implementation:

- persist scanned markdown sections into the index DB as queryable units
- add a section-level FTS path so sections can retrieve and boost their parent artifacts
- move markdown section extraction out of retrieval-only code into a shared package used by scan/index and retrieval
- link persisted todos and criteria to their enclosing section when line ranges make that deterministic
- preserve OpenSpec bundle/collection hierarchy while allowing child section hits to boost parent files and bundles
- add section-level eval diagnostics for selected-by-section, packed-as-section, full-file fallback, and section precision sampling
- keep test cases and code comments as line-scoped artifacts, but align their metadata shape with the section model where practical

Out of scope for this pass:

- master-data/entity indexing
- embeddings or LLM reranking
- broad code symbol indexing

## Retrieval Model

Use a three-level scoring model:

1. Bundle/collection score
2. File/artifact score
3. Section score

The final candidate remains a retrieval artifact, but it may carry one or more selected section hits.

Example candidate metadata:

- `section_match_count`
- `section_match_headings`
- `section_match_ranges`
- `section_match_score`
- `section_match_terms`
- `section_retrieval_mode=section_aware`

The context renderer can continue using the existing section packing path, but retrieval should now know which sections caused selection.

## Index Persistence Model

Add first-class indexed sections instead of extracting sections only after retrieval.

Proposed tables:

```sql
artifact_sections(
  id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  source_path TEXT NOT NULL,
  heading_path TEXT NOT NULL,
  heading_depth INTEGER NOT NULL,
  start_line INTEGER NOT NULL,
  end_line INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  token_estimate INTEGER NOT NULL,
  section_kind TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}'
)
```

```sql
artifact_sections_fts(
  section_id UNINDEXED,
  artifact_id UNINDEXED,
  heading_path,
  title,
  body
)
```

Section IDs should be deterministic for a revision, heading path, and line range. Re-indexing the same revision should replace stale section rows for that revision.

Indexing behavior:

- extract sections immediately after artifact revision content is available
- populate `artifact_sections` and section FTS in the same scan transaction as the parent artifact when possible
- keep parent artifact body FTS unchanged for backward compatibility
- store line ranges so retrieval output can cite exact evidence
- store inherited metadata such as lane, subtype, lifecycle, authority, OpenSpec role, and parent bundle id when available
- key invalidation off the artifact revision/content hash, not the binary version

The first implementation should avoid a separate section cache. The DB itself is the durable query source.

## Upward Relevance Propagation

Add an explicit child-to-parent relevance rule:

```text
relevant section -> parent file/artifact boost -> bundle/collection boost when applicable
```

Sections are retrieval evidence; files remain artifact identity.

This means:

- a strong section hit can make the parent artifact eligible even when the whole file has weak file-level score
- the parent artifact still counts for file-level recall and artifact identity
- context packing should prefer the matched section rather than forcing full-file inclusion
- bundle/collection parents can receive a bounded boost from strong child section hits
- artifact reasons should name the section evidence, including heading path and line range

Do not apply the reverse rule broadly. A relevant parent file does not make every section relevant. Parent-to-child inclusion should remain selective and query/budget aware.

Ranking implication:

- one highly relevant section should outrank many weak generic file/body matches
- parent authority can amplify a section hit, but parent authority alone should not rescue a weak section match
- section hits in stale/superseded/archive artifacts inherit the same lifecycle penalties as their parent unless the query asks for history

Eval implication:

- section-selected parent artifacts count toward existing file-level recall
- section hit details are additional evidence for precision and auditability
- section-aware metrics should distinguish `selected_by_section_hit` from `packed_as_section`

## Section Units

Initial section unit fields:

- stable section id
- parent artifact id when available
- artifact revision/content hash
- source path
- heading path
- heading depth
- line range
- title text
- body excerpt
- body token estimate
- title terms
- body terms
- checkbox/task terms
- requirement cue terms
- code block ratio
- markdown links
- inherited artifact metadata: mode, subtype, authority, lifecycle, OpenSpec role

Extraction should stay deterministic and markdown-native. The first implementation should lift existing markdown section splitting logic into a shared helper used by both scan/index and retrieval.

Todos and criteria:

- `artifact_todos` and `artifact_criteria` should keep their current tables
- when their line range falls inside a persisted section, store the enclosing section id when feasible
- do not block section indexing on this linkage; treat it as an incremental enrichment

Test cases and code comments:

- keep them as first-class line-scoped artifacts rather than forcing them into markdown sections
- align common metadata names where practical: source path, line range, title, token estimate, parent artifact id
- retrieval may score them with the same section-like evidence model, but they do not need to live in `artifact_sections` in v0

## Section Scoring

Score sections with conservative deterministic features:

- query term match in heading path
- identifier-like term match in heading/body
- query phrase match in heading/body
- artifact subtype/lane matches query intent
- requirement cues: `MUST`, `SHALL`, `Acceptance Criteria`, `Scenario`, `Given/When/Then`
- design cues: `Decision`, `Rationale`, `Alternatives`, `Consequences`
- plan cues: `Tasks`, `Implementation`, `Next Steps`, `Progress`
- test behavior cues when the section belongs to test-case artifacts
- authority prior inherited from parent artifact

Penalty features:

- section is mostly code block and query is not code/API specific
- archive/stale/superseded lifecycle without lifecycle query intent
- generated/template/example section without matching query intent
- generic heading only, such as `Overview`, with no body-specific match

## Selection Policy

1. Score all file/artifact candidates as today.
2. Query section FTS/scoring for query-relevant section candidates.
3. Merge section candidates with file/artifact candidates through parent artifact id.
4. Boost parent artifact score when a section score is strong.
5. Admit parent artifacts that would fail file-level threshold but have high-confidence section hits.
6. Preserve file-level exact matches and OpenSpec relationship expansion.
7. Pack selected section hits first; fall back to full file only when the file is short or section confidence is weak.
8. Propagate strong section hits upward to parent bundle/collection artifacts when the relationship is explicit and query mode benefits from the parent context.

Sparse repos must still return plausible file-level matches. Section-aware retrieval should be additive, not a hard gate.

## Eval Strategy

Use the lightweight cached eval loop first.

Runs:

- baseline: current cached dev tier
- experiment: section-aware retrieval enabled
- optional ablation: section-aware scoring enabled but full-file packing preserved, to separate ranking from packing effects

Metrics:

- mean artifact precision
- mean artifact recall
- must-have recall
- context sufficiency pass rate
- token reduction vs full planning
- packed section count
- full-file fallback count
- section-selected artifact count
- section-query candidate count
- section-selected parent artifact count
- section-to-bundle propagation count
- section precision spot-check sample output
- missed-after-discovery count
- low-precision-sufficient cases

Do not tune against lockbox/validation before dev-tier evidence is stable.

## Lightweight Eval Command Shape

Expected local dev loop:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\run-real50-eval.ps1 `
  -Tier dev `
  -Parallelism 4 `
  -IncludeTests `
  -IncludeCodeComments `
  -EvalMaxSourceFiles 1000 `
  -EvalMaxTestCaseArtifacts 500 `
  -EvalMaxCodeComments 100 `
  -EvalMaxCaseSeconds 60
```

The section-aware experiment should be controlled by a feature flag until promoted, for example:

- `--section-aware-retrieval`
- or an internal retriever option used by eval first

## Implementation Plan

1. Inventory existing section packing helpers and move reusable markdown section extraction into a shared internal package.
2. Add section unit type and deterministic extraction for markdown-backed artifacts.
3. Add `artifact_sections` and section FTS schema/migrations.
4. Persist sections during scan/index after artifact revision content is available.
5. Replace stale section rows for a revision when the parent artifact content hash changes.
6. Add optional section id linkage for todos/criteria when line ranges make it deterministic.
7. Add section query/scoring beside the existing file scoring path in `internal/retrieval`.
8. Add parent score boosts and section-hit metadata.
9. Add bounded OpenSpec bundle/collection propagation from explicit child section hits.
10. Ensure selected section metadata flows into `artifact_reasons`.
11. Ensure context packing prefers section hits already chosen by retrieval.
12. Add eval fields for section-aware artifacts, section-query candidates, propagated bundle hits, and full-file fallbacks.
13. Run dev-tier baseline vs experiment.
14. Promote only if metrics improve or precision/sufficiency improves without material recall loss.

## Coordination With Indexing-Speed Work

Avoid touching:

- adapter file walking
- scanner parallelization
- SQLite write batching
- comment/test extraction internals

Allowed touch points:

- retrieval candidate metadata
- retrieval scoring
- eval result schema
- context packing behavior after candidates already exist

If the parallel speed work changes candidate shape, rebase this plan onto that shared inventory/extraction model.

## Auditable Success Criteria

- `ds eval --json` can report how many returned artifacts were selected through section-aware evidence.
- The SQLite index contains persisted section rows for markdown artifacts with headings.
- Section FTS can retrieve a section by heading/body terms without requiring the parent artifact to win file-level retrieval first.
- Every section-selected artifact includes source path, heading path, and line range in metadata or reasons.
- Section-selected parent artifacts count toward file-level recall while preserving section evidence in reasons.
- Upward relevance propagation boosts parent file/artifact ranking without forcing full-file inclusion.
- Bundle/collection boosts from section hits are bounded and only happen through explicit parent-child relationships.
- Existing `artifact_todos` and `artifact_criteria` behavior does not regress; enclosing section ids are added when deterministic.
- Test-case and code-comment artifacts keep their current behavior while exposing compatible line-range/title metadata for section-like scoring.
- Dev-tier experiment improves mean artifact precision or graded precision versus baseline.
- Must-have recall does not regress by more than 2 percentage points on the dev tier.
- Context sufficiency does not regress on the dev tier.
- Full-file fallback count is visible in JSON output.
- Section-query candidate count and selected-by-section count are visible in JSON output.
- Short files still behave as whole-file artifacts.
- No rule contains repo names, mined-case-specific phrases, or one-off path literals.
- Unit tests cover nested headings, heading/body term matches, generic heading penalties, code-block-heavy sections, and full-file fallback.
- A cached eval rerun can compare baseline vs section-aware retrieval without rebuilding the index.

## Rollback Criteria

- Section-aware scoring hides must-have artifacts.
- Section-selected artifacts improve precision only by suppressing recall.
- Explanations become less clear than file-level reasons.
- Runtime increases enough that dev-tier eval no longer fits a short feedback loop.
