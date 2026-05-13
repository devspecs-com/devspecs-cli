# Tasks: Improve Retrieval Quality

## Eval Metrics

- [x] Support object-form `expected_relevant` entries with `path` and `importance`.
- [x] Preserve backward compatibility for string-form `expected_relevant` entries.
- [x] Report overall recall plus `must`, `helpful`, and `background` recall.
- [x] Add `success_criteria` parsing for terms and artifacts.
- [x] Report per-case sufficiency pass/fail.
- [x] Report aggregate sufficiency pass rate.
- [x] Add tests for importance weighting and sufficiency failures.
- [x] Persist a timestamped JSON result file for each eval run.

## Fixture Updates

- [x] Mark current seed cases with `must`, `helpful`, and `background` relevance.
- [x] Add sufficiency criteria for the highest-value cases.
- [x] Keep known semantic failures visible; do not rewrite cases to hide them.

## Retrieval Improvements

- [ ] Add identifier-aware tokenization and matching.
- [ ] Search identifier matches in path, title, body, extracted tasks, and source candidates.
- [ ] Add dated filename/slug matching tests.
- [ ] Add OpenSpec bundle representation for proposal/design/tasks/spec deltas.
- [ ] Include OpenSpec design/spec deltas for implementation-context queries.
- [ ] Add general authority/lifecycle scoring.
- [ ] Add deterministic query intent classification.
- [x] Add score reason collection in eval JSON.

## Validation

- [x] Run `ds eval ./fixtures/agentic-saas-fragmented`.
- [x] Compare token reduction, overall recall, must-have recall, precision, and sufficiency rate.
- [ ] Verify precision improves without collapsing token reduction.
- [ ] Keep `eval_stage: seed_smoke` until benchmark fixtures are locked.
