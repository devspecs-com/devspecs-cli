# Task waspflow-shell-support W01 Plan

## Goal
Pin Waspflow baseline and define ground-truth quality.

## Why This Slice Exists
The initial probe shows a measurable substrate miss: the cold index contains design documents but none of the shell implementation or behavioral verification. Before changing admission or ranking, preserve the raw outputs and define what a useful result must contain.

## Inputs
- Waspflow commit `3d9634af32a5d8791671e0a320eedc2047141a2c` with full Git history.
- Released `v1.3.0`, the current v1.4.0 release-candidate `main`, and the exact candidate binary.
- Independent empty `DEVSPECS_HOME` directories per command and binary.
- Queries: `worker lane lifecycle`, `provider orchestration`, `verification escalation`, `federation design`, and `model selection policy`.
- Task probe: `add a provider while preserving verification, selection, billing, and event behavior`.

## Work
- Record raw and normalized JSON/text for cold `scan`, `recent`, `map`, each `find`, and the task probe.
- Record wall time, phase timing, artifact kinds, selected paths, roles, warnings, and excluded noise.
- Create a human-reviewed oracle that names required, useful, neutral, and harmful evidence per command/query.
- Pin two unrelated shell-first repositories and exact commits for cross-repo validation.
- Capture current Git-derived `recent` topics as a preservation fixture.

## Acceptance
- The baseline can be replayed from one documented command with no warm index reuse.
- The oracle requires implementation and behavior-test evidence where the query asks for behavior.
- The oracle does not require Waspflow-specific words or exact ranks when semantically equivalent evidence is present.
- Current misses are classified, not merely counted.
- No production behavior changes in this slice.

## Decision Gates
- Promote: baseline, oracle, fixtures, and timing method are reproducible and reviewed.
- Improve: outputs exist but quality labels or timing isolation remain ambiguous.
- Rework: the oracle overfits Waspflow paths or cannot distinguish useful from merely matching output.
- Block: the pinned repository or release binary cannot be reproduced.
