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
- [x] Make indexed SQLite corpus eval the default path.
- [x] Keep raw filesystem eval as a diagnostic flag.

## Fixture Updates

- [x] Mark current seed cases with `must`, `helpful`, and `background` relevance.
- [x] Add sufficiency criteria for the highest-value cases.
- [x] Keep known semantic failures visible; do not rewrite cases to hide them.

## Retrieval Improvements

- [x] Use `docs/plans/2026-05-13-retrieval-improvement-test-index.md` to plan each retrieval experiment before implementation.
- [x] Use `docs/plans/2026-05-13-eval-to-cli-integration-plan.md` to keep eval work connected to indexed and live CLI paths.
- [ ] Extract shared retrieval candidate/retriever logic out of `internal/evalharness`.
- [ ] Upgrade `ds find` to use or prepare for shared indexed retrieval with reasons.
- [ ] Add query-focused `ds resume <query>` over indexed candidates.
- [ ] Add live-command eval mode for the existing command path.
- [ ] Decide whether a public `ds pack <query>` command is needed after existing workflows are measured.
- [ ] Decide keep/deprecate/remove path for `ds context <id>`.
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
