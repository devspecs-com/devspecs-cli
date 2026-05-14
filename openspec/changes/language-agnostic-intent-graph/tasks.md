# Tasks: Language-Agnostic Intent Graph

## Planning And Architecture

- [x] Create language-agnostic intent graph plan.
- [x] Create OpenSpec proposal, design, tasks, and spec deltas.
- [ ] Decide the initial in-memory graph package boundary.
- [ ] Define artifact, section, entity, and edge structs.
- [ ] Define retrieval reason schema for graph/entity/section matches.

## Universal Extraction

- [ ] Add language-neutral identifier normalization.
- [ ] Preserve snake_case, kebab-case, dotted, slash-path, and CamelCase variants.
- [ ] Extract markdown headings and section spans.
- [ ] Extract YAML frontmatter metadata.
- [ ] Extract checkboxes and task-like bullets.
- [ ] Extract status and lifecycle labels.
- [ ] Extract path-shaped prose references.
- [ ] Add tests for language-neutral extraction.

## Artifact-Type Adapters

- [ ] Add OpenSpec graph adapter for proposal/design/tasks/spec deltas.
- [ ] Add ADR graph adapter for status, decision, context, consequences, and supersession.
- [ ] Add PRD graph adapter for product background and requirements.
- [ ] Add plan graph adapter for decisions, deferred work, risks, and rationale.
- [ ] Add agent-note graph adapter for follow-up, blockers, and continuation hints.

## Graph Edges And Ranking

- [ ] Build deterministic edges between artifacts, sections, and entities.
- [ ] Add local alias expansion from exact variants and bounded co-occurrence.
- [ ] Add query intent classification over graph entities.
- [ ] Add graph-aware scoring with authority, lifecycle, section role, and edge strength.
- [ ] Add bundle expansion through strong edges, especially OpenSpec companions.
- [ ] Add noise caps so generic terms do not dominate precision.

## Source Context

- [ ] Add generic source/code entity extraction by regex.
- [ ] Represent source symbols as language-neutral `source_symbol` or `code_entity`.
- [ ] Add at least one non-TypeScript source/context eval case.
- [ ] Keep AST or tree-sitter extraction optional and out of the critical path.

## Eval And Validation

- [ ] Use `docs/plans/2026-05-13-retrieval-improvement-test-index.md` to choose keep / revise / reject decisions for each graph improvement.
- [ ] Add eval cases for language-neutral source/context retrieval.
- [ ] Add eval cases for section-role retrieval.
- [ ] Add eval cases for alias expansion from local evidence.
- [ ] Compare timestamped eval results before and after each heuristic.
- [ ] Verify must-have recall and sufficiency improve without collapsing token reduction.
- [ ] Keep `eval_stage: seed_smoke` until benchmark fixtures are locked.
