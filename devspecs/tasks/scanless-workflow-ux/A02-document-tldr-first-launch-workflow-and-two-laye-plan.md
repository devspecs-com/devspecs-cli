# Task scanless-workflow-ux A02 Plan

## Goal
Document tldr-first launch workflow and two-layer PLAN-to-task model

## Dogfood Trigger
ScopeLab feedback called `ds tldr` the best entry point and warned that DevSpecs should not pretend to replace existing M00/T00/EV00-style plan indexes. Launch docs should make the model explicit: canonical intent/spec artifacts remain the source of truth, while `devspecs/tasks/*` provides bounded execution slices and receipts.

## Expected Change Surface
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`
- docs site content if this slice runs in the docs repo later
- `internal/commands/tldr.go` only if wording needs CLI-surface alignment

## Required Content
- Start agent sessions with `ds tldr`.
- Use `ds find`/`ds map` to route to existing owner decision docs, not to replace reading them.
- Explain the two-layer model:
  - `PLAN-*`, ADRs, PRDs, RFCs, decision memos, and north-star docs are canonical intent/spec artifacts.
  - `devspecs/tasks/<task-id>/` is an execution workspace for bounded slices, prompts, checkpoints, and result receipts.
- Tell users not to duplicate gates between canonical plans and task workspaces; link them instead.
- Explain when to use full `ds task` versus `ds task quick`.
- Add install/onboarding note: restart terminal, shell, or IDE after install if `ds` is not found.

## Out-of-Scope Areas
- Retrieval ranking changes; those belong to F.
- Package-manager self-update behavior; that belongs to G.
- Reworking task artifact format.

## Success Criteria
- [ ] README or docs make `ds tldr` the visible first command for agents.
- [ ] The two-layer PLAN/spec-to-task model is explicit enough that agents know which artifact is canonical.
- [ ] Install docs mention restarting shell/IDE after install or upgrade.
- [ ] Full task versus quick task guidance matches launch positioning.

## Decision Gates
- Promote: docs reduce launch confusion without adding ceremony.
- Improve: docs are directionally correct but need shorter examples.
- Rework: docs imply DevSpecs replaces owner decision docs.
- Rollback: docs make task workspaces feel mandatory for every tiny change.
