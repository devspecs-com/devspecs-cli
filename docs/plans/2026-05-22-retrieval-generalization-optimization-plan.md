# Retrieval Generalization Optimization Plan

Date: 2026-05-22

## Goal

Improve first-index retrieval precision on the real dev50 set without baking in repo-specific behavior or miner-label artifacts.

## Current Signal

The expanded real dev set exposes a general retrieval failure shape:

- Generic body terms such as `document`, `requirements`, `instructions`, and `user` can swamp exact path/title matches.
- Classified artifacts are not always scored according to their classifier role when their path is non-standard.
- Protocol and template artifacts are useful when explicitly requested, but they create noise when only generic terms match.
- Agent-produced plans should usually share plan retrieval behavior with generic markdown plans unless they have a stable structural format.

## Experiment

Implement only broad, reusable retrieval changes:

- Add phrase bridges for common artifact names and acronyms, such as Product Requirements Document to `prd` and Architecture Decision Record to `adr`.
- Score candidates using classifier metadata and kind/subtype in addition to path layout.
- Treat protocol/template/model lanes as mode-gated unless the query asks for that mode.
- Prefer shallow repository-wide instruction files for repository/project-wide instruction queries, while allowing nested instruction files when the query names a module or exact subject.
- Lower weight for generic role words so they do not defeat subject/path/title matches.

## Non-Goals

- Do not add repository names, known sample paths, or exact mined-case strings.
- Do not split Codex/Claude/Cursor/generic plans into separate retrieval roles unless a stable structural format emerges.
- Do not tune against validation or lockbox labels in this pass.

## Success Criteria

- Canonical eval must not regress on must-have recall or classifier fixture correctness.
- Real dev50 completed-case precision should improve materially while keeping must-have recall near the current level.
- Any recall loss must be explainable as removal of weak generic-noise hits, not missed exact artifact formats.
- The implementation must be auditable as general scoring behavior.

## Rollback Criteria

- Precision improves only by dropping must-have recall below 0.80 on the completed real dev50 cases.
- Canonical fixture sufficiency or must-have recall regresses.
- The diff introduces repo/sample-specific literals or generator-specific format claims without structural evidence.
