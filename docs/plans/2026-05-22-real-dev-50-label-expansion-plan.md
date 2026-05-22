# Real Dev 50 Label Expansion Plan

Date: 2026-05-22

## Goal

Expand the ignored real-repository retrieval development set from 15 first-pass cases toward 50 cases, while keeping label risk explicit and avoiding lockbox contamination.

## Risk Position

OpenSpec classification labels are low risk because layout evidence is strong. Retrieval labels are higher risk because the reviewer must decide which artifacts are must-have, helpful, background, or distracting for a task query. Risk is highest for broad plans, roadmaps, design docs, and product specs; medium for RFC/enhancement/proposal families; and lower for ADRs with clear path plus status/context/decision structure.

## Expansion Strategy

Use the existing ignored dev split only:

- Source root: `devspecs-sample-miner/_ignore/devspecs-cli-intent-discovery-dev50-20260520-175112/repos`
- Output root: `devspecs-sample-miner/_ignore/real-retrieval-dev50-YYYYMMDD-HHMMSS`
- Copy selected repo fixtures into `output/repos/<repo>`.
- Preserve existing reviewed `cases.yaml` where present.
- Add first-pass Codex labels to additional repos with clear candidate artifacts.

Target mix:

- 10 OpenSpec/bundle-aware cases.
- 10 RFC/enhancement/proposal cases.
- 10 ADR/architecture decision cases.
- 10 plan/roadmap/design-doc cases.
- 5 PRD/product-spec cases.
- 5 agent-plan/protocol-adjacent cases.

## Labeling Rules

- `must`: required to avoid implementing the wrong behavior.
- `helpful`: useful context but not required.
- `background`: durable context or lineage.
- `expected_excluded`: near-miss artifacts that should not be retrieved for this query.

Each case must include at least one `must` artifact. Prefer 2-4 expected artifacts and 1-3 explicit exclusions. Labels are first-pass and must be treated as optimization/dev labels until human-reviewed.

## Acceptance Criteria

- The expanded set contains at least 50 cases.
- At least 12 repositories are represented.
- No lockbox or validation repos are copied into the dev set.
- `ds eval <output-root> --first-index-report --batch-fixtures --json --no-save` runs and emits aggregate metrics.
- The output README records source split, label status, repo/case count, and the measured aggregate.

## Guardrails

- Do not tune on validation or lockbox while creating this set.
- Do not add repo-specific classifier or retrieval rules from this labeling pass.
- Treat surprising high/low metrics as review prompts, not proof.
- Before public claims, create a repo-disjoint validation set and manually audit a sample of passing cases plus all failing/low-precision cases.
