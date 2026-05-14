# Tasks: Add Adapter Classifier Pipeline

## Planning

- [x] Create adapter classifier pipeline plan.
- [x] Add OpenSpec proposal/design/tasks/spec.
- [x] Document branch and incremental commit discipline for `feat/adapter-classifier-pipeline`.
- [x] Decide package boundary for classifier contracts: `internal/classify`.
- [ ] Decide whether classifier eval lives in `ds eval` or a focused internal submode.

## Implementation Notes

- Phase 0 is implemented as `internal/classify`.
- Phase 0 is intentionally contract-only: no scan, DB, retrieval, or command behavior changes.
- Initial classifier goldens live at `fixtures/agentic-saas-fragmented/classifier_cases.yaml`.
- Real sample intake template lives at `testdata/classifier-samples/provenance-template.yaml`.

## Phase 0: Contract And Goldens

- [x] Define classifier candidate, features, classification, reason, and resolution types.
- [x] Define container and document candidate scopes.
- [x] Define container expansion output for child document candidates.
- [x] Define classifier configuration schema.
- [x] Define built-in classifier profile shape.
- [x] Define repo/user override shape.
- [x] Confirm configuration supports all documented built-in document models and subformat/family models.
- [x] Define confidence and ambiguity semantics.
- [x] Define optional classifier subformat/family semantics.
- [x] Define reason vocabulary for positive and negative evidence.
- [x] Add classifier fixture/golden format.
- [x] Add initial goldens from the existing seed fixture.
- [x] Add real-sample provenance template for GitHub-mined files.

## Phase 1: Universal Feature Extraction

- [ ] Extract path tokens and filename slugs.
- [ ] Extract dated filename tokens.
- [ ] Extract frontmatter.
- [ ] Extract title and markdown headings.
- [ ] Extract markdown section spans.
- [ ] Extract checklist density.
- [ ] Extract status/lifecycle phrases.
- [ ] Extract identifier-shaped terms.
- [ ] Extract path-shaped references.
- [ ] Extract generated/changelog/stale markers.
- [ ] Add deterministic unit tests for feature extraction.

## Phase 2: Classifiers

- [ ] Add OpenSpec classifier.
- [ ] Add OpenSpec container expansion for proposal/design/tasks/spec child artifacts.
- [ ] Add ADR classifier.
- [ ] Add ADR Nygard subformat evidence.
- [ ] Add ADR MADR subformat evidence.
- [ ] Add ADR Y-Statement subformat evidence.
- [ ] Add PRD classifier.
- [ ] Add RFC/proposal section-pattern classifier.
- [ ] Add plan classifier.
- [ ] Add agent-note classifier.
- [ ] Add generic markdown fallback classifier.
- [ ] Add tests for ambiguous documents and generic fallback.
- [ ] Add tests for negative evidence.

## Phase 3: Classifier Eval

- [ ] Report discovery coverage.
- [ ] Report classifier accuracy by model.
- [ ] Report classifier accuracy by subformat/family where expected labels exist.
- [ ] Report false positives and false negatives.
- [ ] Report ambiguity rate.
- [ ] Report generic fallback rate.
- [ ] Report reject rate.
- [ ] Report top confusion pairs.
- [ ] Report reason coverage.
- [ ] Persist timestamped classifier eval JSON.

## Phase 4: Scan Integration

- [ ] Add broad safe candidate discovery behind conservative filters.
- [ ] Run classifier resolver before parse/index.
- [ ] Preserve existing configured-path adapter behavior.
- [ ] Store classifier metadata in extracted JSON.
- [ ] Include classifier metadata in indexed retrieval candidates.
- [ ] Confirm `ds scan` remains deterministic and local-only.

## Phase 5: Retrieval Integration

- [ ] Use classifier authority and lifecycle in ranking.
- [ ] Use classifier ambiguity/generic fallback in ranking.
- [ ] Use classifier artifact type for query-intent preferences.
- [ ] Add eval reasons showing classifier influence.
- [ ] Compare indexed and live-command eval before/after.
- [ ] Keep or revise based on token reduction, recall, must-have recall, precision, and sufficiency.

## Deferred

- [ ] Add normalized classifier-result tables.
- [ ] Add section/entity/edge tables.
- [ ] Add optional source-code AST classifiers.
- [ ] Add LLM judging.
