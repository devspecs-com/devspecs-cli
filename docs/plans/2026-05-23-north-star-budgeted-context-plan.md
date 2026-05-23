# North Star Budgeted Context Eval Plan

Date: 2026-05-23

## Goal

Move the eval closer to the product promise: devspecs should give agents enough useful engineering intent context with far fewer tokens than broad repository search.

The near-term scorecard is trust-first:

- `85%+` context-sufficient packs
- `90%+` must-have recall
- `90-97%` context reduction versus query-file baseline

We should prefer a trustworthy `95%` reduction with `90%` recall over a brittle `99%` reduction with materially lower recall.

The eval should continue reporting precision/recall, but the headline should make objective utility visible:

- context sufficiency
- must-have recall
- must-hit ranking
- sufficiency under token budgets
- token reduction versus a query-matched file baseline
- hard-negative/noise pressure

## Work Items

1. Promote north-star reporting:
   - mean and median token reduction versus query-file baseline
   - sufficiency within 1k, 2k, 4k, and 8k token budgets
   - must-hit@1/3/5/10 and first useful/must rank
   - hard-negative and unlabeled artifact counts
   - keep exact and graded precision as guardrails
2. Add an opt-in trust-first budgeted context packer:
   - preserve existing retrieval admission
   - preserve balanced evidence as rerank-only
   - trim the already-ranked artifact list to a token budget only as an experiment
   - record pre-budget tokens and dropped artifacts
   - do not make this default until eval wins are clear
   - evaluate larger/trust-first budgets before treating smaller packs as wins
3. Add insufficient-case diagnostics:
   - expected artifact missing from corpus
   - expected artifact indexed/discovered but not retrieved
   - forbidden artifact/term present
   - missing terms despite retrieved artifacts
   - budget/other residual bucket
4. Preserve balanced evidence as ranking:
   - no evidence-aware admission in this pass
   - balanced mode should only affect ordering unless combined with the explicit budgeted packer experiment

## Auditable Success Criteria

- `ds eval` JSON summary includes mean token reduction versus query-file baseline.
- `ds eval` case JSON includes pre-budget token count and dropped artifacts when budgeted packing is enabled.
- `ds eval --experimental-budgeted-packing --eval-context-token-budget 8192` keeps returned contexts at or under the budget whenever candidate granularity allows it.
- The real50 runner aggregate includes:
  - weighted mean/median query-baseline token reduction
  - sufficiency@1024/2048/4096/8192
  - must-hit@1/3/5/10
  - total hard-negative and unlabeled retrieved artifacts
  - insufficient-case diagnosis counts and examples
- Unit tests cover budgeted packing trimming and metric visibility.
- Full real50 control and budgeted treatment complete with all repositories represented.
- Treatment should move toward the trust-first scorecard before promotion:
  - `85%+` context sufficiency
  - `90%+` must-have recall
  - `90-97%` query-baseline token reduction
- Any compression gain that lowers must-have recall or sufficiency is not a win.

## Guardrails

- Do not remove hard negatives or broad labels to improve metrics.
- Do not use repository-specific paths or mined-case literals in retrieval logic.
- Do not treat exact precision as the sole north-star metric.
- Do not promote the budgeted packer to default behavior in this pass.

## Results

Implemented on 2026-05-23:

- Summary-level query-baseline token reduction is now emitted by `ds eval`.
- The real50 runner aggregate now surfaces:
  - weighted mean and median query-baseline token reduction
  - sufficiency at 1024/2048/4096/8192 tokens
  - must-hit@1/3/5/10 and first-rank metrics
  - hard-negative and unlabeled artifact pressure
  - insufficient-case diagnosis categories and examples
- Added opt-in `--experimental-budgeted-packing` with `--eval-context-token-budget`; the default budget is trust-first `8192` tokens.
- Balanced evidence remains rerank-only unless the budgeted packer is explicitly enabled.

Validation:

- `go test ./internal/evalharness ./internal/commands -count=1`
- Dev tier control and budget treatment completed with 12 repos / 31 cases / 0 failures.
- Full real50 control and budget treatment completed with 47 repos / 116 cases / 0 failures.

Full real50 balanced control:

- Sufficiency: `0.7845`
- Must-have recall: `0.7931`
- Query-baseline reduction mean: `0.9685`
- Query-baseline reduction weighted median: `0.9675`
- Sufficiency@8192: `0.7500`
- Must-hit@3: `0.8017`
- Hard negatives: `16`
- Unlabeled retrieved artifacts: `250`
- Insufficient-case diagnostics: `18` missed-after-discovery, `8` missing-from-corpus, `1` missing-terms

Full real50 8192 budgeted treatment:

- Sufficiency: `0.7845`
- Must-have recall: `0.7931`
- Query-baseline reduction mean: `0.9686`
- Query-baseline reduction weighted median: `0.9676`
- Sufficiency@8192: `0.7845`
- Must-hit@3: `0.8017`
- Hard negatives: `16`
- Unlabeled retrieved artifacts: `244`
- Budget-dropped artifacts: `6`

Interpretation:

- Compression is already in the desired `90-97%` band.
- The current gap to the trust-first target is not pack size; it is missing must-have retrieval/indexing.
- The largest visible miss bucket is missed-after-discovery, especially line-scoped test artifacts and standard agent/spec template files.
- Budgeted packing is useful instrumentation and may slightly reduce noise, but it does not address the main recall/sufficiency gap yet.
