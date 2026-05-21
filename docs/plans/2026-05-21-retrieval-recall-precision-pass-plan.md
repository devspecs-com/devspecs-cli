# Retrieval Recall and Precision Pass Plan

Date: 2026-05-21

## Context

The current indexed eval has strong discovery coverage, but several expected artifacts are discovered and then lose ranking. The remaining work should improve context selection, not broaden scanning.

Current smoke baseline:

- Mean token reduction: 90.1%
- Mean artifact recall: 80.3%
- Mean must-have recall: 93.2%
- Mean artifact precision: 76.1%
- Context sufficiency: 9/11

Observed gaps:

- Indexed eval candidates do not include artifact link metadata, while live command candidates do.
- OpenSpec bundle containers are often selected as context even when the useful context is in child files.
- OpenSpec sibling files such as `tasks.md` and spec deltas can miss implementation queries.
- Identifier/source queries can miss helpful product intent context or admit structural containers.

## Goals

- Improve recall and precision together on the current first-index smoke.
- Keep token reduction near the current range.
- Use general signals: artifact links, source intent, structural containers, role-specific OpenSpec children, and path/title/body evidence.
- Keep changes easy to roll back if validation sets disagree.

## Non-Goals

- Do not add per-repository or per-file exceptions.
- Do not broaden discovery in this pass.
- Do not change classifier labels.
- Do not claim public accuracy from this smoke alone.

## Plan

1. Align eval retrieval candidates with live command candidates by carrying link metadata from the SQLite index.
2. Treat OpenSpec bundle containers as structural context by default: useful for expansion and explicit bundle queries, but not a strong standalone match for ordinary implementation/source/RFC queries.
3. Expand OpenSpec siblings through recorded links for context-heavy queries, while avoiding unconditional parent bundle inclusion.
4. Add synthetic retrieval unit tests that cover:
   - child-to-sibling OpenSpec expansion without parent bundle noise
   - explicit bundle queries still retrieving bundle context
   - source/identifier queries avoiding structural OpenSpec containers
5. Run the same eval and test gates before/after:
   - `go run -buildvcs=false ./cmd/ds eval fixtures/agentic-saas-fragmented --json --no-save`
   - `go run -buildvcs=false ./cmd/ds eval fixtures/agentic-saas-fragmented --no-save`
   - `go test -buildvcs=false -count=1 ./internal/retrieval ./internal/evalharness ./internal/commands`

## Auditable Success Criteria

- Mean artifact precision improves over 76.1%.
- Mean artifact recall does not drop below 80.3%.
- Mean must-have recall stays at or above 93.2%.
- Context sufficiency improves above 9/11 or at least does not regress.
- OpenSpec bundle recall and child-role recall remain 100% on the smoke.
- No new per-fixture filenames are introduced into retrieval scoring.

## Implemented Result

Final local indexed eval after this pass:

- Mean token reduction: 93.8%
- Mean artifact recall: 89.0%
- Mean must-have recall: 97.7%
- Mean artifact precision: 88.0%
- Context sufficiency: 10/11
- Discovery coverage: 100.0%
- OpenSpec bundle recall: 100.0%
- OpenSpec child-role recall: 100.0%

Validation-50 scan/index check:

- Repos scanned: 50
- Non-zero exits: 0
- Markdown count delta versus prior current rerun: 0
- OpenSpec count delta versus prior current rerun: 0
- Output: `devspecs-sample-miner/_ignore/retrieval-validation-scan-20260522-000746`

This validation check confirms first-index coverage did not move on the real-repo validation split. It does not measure retrieval precision because validation-50 does not yet contain manually labeled retrieval tasks.
