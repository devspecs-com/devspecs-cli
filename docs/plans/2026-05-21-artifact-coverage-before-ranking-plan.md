# Artifact Coverage Before Ranking Plan

## Goal

Improve first-index artifact coverage before further retrieval/ranking tuning.

The current eval shows strong OpenSpec, ADR, RFC, and PRD behavior, but several mined or common repo-local artifact classes are only partially represented:

- Spec Kit feature docs
- BMAD-like story artifacts
- Cursor, Claude, Codex, and generic plan docs
- standard root markdown intent files such as `ROADMAP.md`, `PLAN.md`, and `DESIGN.md`

## Experiment Scope

Patch general discovery and classification signals, not repo-specific sample names.

### In Scope

- Root-file discovery for common standard intent docs.
- Broader default markdown paths for common agent-plan directories.
- Spec Kit profile recognition beyond only `spec.md`.
- Plan classifier families for roadmap and story-shaped artifacts.
- Agent-note classifier signals for Cursor, Claude, and Codex plan paths.
- Focused tests proving the new coverage does not depend on one mined file.

### Out Of Scope

- Retrieval/ranking score changes.
- First-class Spec Kit bundles.
- First-class BMAD strict model.
- Procedure/model/template lane promotion.

## Success Criteria

- `ROADMAP.md`, `PLAN.md`, `DESIGN.md`, and `ARCHITECTURE.md` are discovered by default when present at repo root.
- Spec Kit `specs/<feature>/{spec,plan,tasks}.md` files receive `format_profile=speckit` and share a feature `layout_group`.
- `.cursor/plans/**`, `.claude/**`, and `.codex/**` candidates can classify as `agent_note` when handoff/continuation signals are present.
- BMAD-like `*.story.md` files with story/task/acceptance structure classify as `plan` with a story family.
- Root roadmap files classify as `plan` with a roadmap family.
- Existing classifier and indexed retrieval eval smoke does not regress materially.

## Validation Commands

```sh
go test -buildvcs=false ./internal/format ./internal/adapters/markdown ./internal/classify ./internal/discover ./internal/scan
go run -buildvcs=false ./cmd/ds eval fixtures/agentic-saas-fragmented --classifier --no-save
go run -buildvcs=false ./cmd/ds eval fixtures/mined-intent-samples --classifier --no-save
go run -buildvcs=false ./cmd/ds eval fixtures/agentic-saas-fragmented --no-save
```
