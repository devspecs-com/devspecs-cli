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
- Phase 1 is implemented in `internal/classify` as pure feature extraction only.
- Phase 2 is implemented as a generic declarative document-model evaluator, not as hard-coded Go classifiers per document type.
- Phase 3 is implemented as a focused classifier eval mode under `ds eval --classifier`.
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

- [x] Extract path tokens and filename slugs.
- [x] Extract dated filename tokens.
- [x] Extract frontmatter.
- [x] Extract title and markdown headings.
- [x] Extract markdown section spans.
- [x] Extract checklist density.
- [x] Extract status/lifecycle phrases.
- [x] Extract identifier-shaped terms.
- [x] Extract path-shaped references.
- [x] Extract generated/changelog/stale markers.
- [x] Add deterministic unit tests for feature extraction.

## Phase 2: Declarative Document Model Evaluator

- [x] Add generic evidence-rule evaluator over `PipelineConfig`.
- [x] Add auditable evidence rule IDs, weights, reasons, and feature predicates.
- [x] Add declarative OpenSpec document model rules.
- [x] Add declarative OpenSpec container expansion for proposal/design/tasks/spec child artifacts.
- [x] Add declarative ADR document model rules.
- [x] Add declarative ADR Nygard subformat evidence.
- [x] Add declarative ADR MADR subformat evidence.
- [x] Add declarative ADR Y-Statement subformat evidence.
- [x] Add declarative PRD document model rules.
- [x] Add declarative RFC/proposal section-pattern rules.
- [x] Add declarative plan document model rules.
- [x] Add declarative agent-note document model rules.
- [x] Add declarative generic markdown fallback rules.
- [x] Add local model definitions that inherit base models and add declarative evidence.
- [x] Add tests for ambiguous documents and generic fallback.
- [x] Add tests for negative evidence.

## Phase 3: Classifier Eval

- [x] Report discovery coverage.
- [x] Report classifier accuracy by model.
- [x] Report classifier accuracy by subformat/family where expected labels exist.
- [x] Report false positives and false negatives.
- [x] Report ambiguity rate.
- [x] Report generic fallback rate.
- [x] Report reject rate.
- [x] Report top confusion pairs.
- [x] Report reason coverage.
- [x] Persist timestamped classifier eval JSON.

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
