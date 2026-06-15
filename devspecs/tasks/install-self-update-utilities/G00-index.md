# Task install-self-update-utilities

## Task
Install and self-update utilities

## Status
packed

## Series
G

## Profile
greenfield

## Created At
2026-06-15T15:09:47Z

## Original Query
Install and self-update utilities

## Track Intent
This is the pre-launch install/update utility track. It exists because first-run PATH friction and release churn make `how do I get current?` a real launch problem.

The goal is a simple `ds update` command that helps users upgrade without pretending DevSpecs owns every package manager. It should detect likely install source when possible, print the right command, and optionally run safe package-manager upgrades only when the path is clear.

## Timing
Pre-launch or early patch. This is not core task intelligence, but it reduces human friction and helps dogfooders retry the full process after release fixes.

## Product Decisions
- `ds update` should be explicit. Do not check the network or latest release on every command.
- Version staleness detection should be lightweight and cached, or only run from `ds update` / `ds version --check`.
- Prefer package-manager-aware guidance over a brittle universal self-mutating binary updater.
- Always document restart shell/IDE after install or upgrade when PATH shims may not be visible.

## Non-Goals
- Do not build an apt repository in this track.
- Do not add background auto-update.
- Do not make update checks noisy during normal agent workflows.
- Do not collect identifying install telemetry as part of update checks.

## Decision Gates
- Promote G01 if `ds update` gives correct guidance for Homebrew, Scoop, Go install, and script/manual users.
- Promote G02 only if staleness checks are cached, explicit, and non-disruptive.
- Promote G03 when install docs cover terminal/IDE restart in the common paths.
- Rework if update UX risks surprising binary mutation.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/install-self-update-utilities`

## Resources
- `task.json`
- `G01-add-ds-update-as-an-explicit-self-update-command-plan.md`
- `G01-add-ds-update-as-an-explicit-self-update-command-result.md`
- `G02-add-lightweight-version-staleness-detection-with-plan.md`
- `G02-add-lightweight-version-staleness-detection-with-result.md`
- `G03-document-restart-shell-or-ide-after-install-and-plan.md`
- `G03-document-restart-shell-or-ide-after-install-and-result.md`

## Task Slices
- G01: Add ds update as an explicit self-update command with package-manager-aware guidance. Plan: `G01-add-ds-update-as-an-explicit-self-update-command-plan.md`. Result: `G01-add-ds-update-as-an-explicit-self-update-command-result.md`.
- G02: Add lightweight version staleness detection without checking on every command. Plan: `G02-add-lightweight-version-staleness-detection-with-plan.md`. Result: `G02-add-lightweight-version-staleness-detection-with-result.md`.
- G03: Document restart shell or IDE after install and upgrade. Plan: `G03-document-restart-shell-or-ide-after-install-and-plan.md`. Result: `G03-document-restart-shell-or-ide-after-install-and-result.md`.

## Relevant Map Areas
No strong map area was inferred from the initial pack.

## Likely Primary Files
None found in the initial preflight.

## Likely Tests
None found in the initial preflight.

## Likely Docs / Plans / Config
None found in the initial preflight.

## Supporting Context
None found in the initial preflight.

## Related Git Receipts
None found from packed paths.

## Noise Risks
None found in the initial preflight.

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Pack completeness is not high; verify the working set before committing to implementation scope.

## Confidence Summary
- Primary file confidence: low
- Test coverage confidence: low
- Docs/config coverage confidence: low
- Git receipt confidence: low
- Noise risk: low
- Pack completeness: low

Why:
- no clear primary implementation file was found
- test companion coverage was not evident from the initial pack

Agent instruction:
Use the evidence to define the first bounded planning artifact, evaluation signal, and next-slice decision before implementation scope expands.

## Suggested Starting Slice
Use `G01-add-ds-update-as-an-explicit-self-update-command-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `G01-add-ds-update-as-an-explicit-self-update-command-result.md` or `ds task checkpoint`.
