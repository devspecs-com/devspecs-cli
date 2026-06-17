# Task cross-track-route-loops

## Task
Cross-track route loop placeholder: preserve ordered multi-track release routes, repair slices, smoke reruns, and later route views without building full orchestration yet

## Status
packed

## Series
N

## Profile
code-change

## Created At
2026-06-17T12:33:30Z

## Original Query
Cross-track route loop placeholder: preserve ordered multi-track release routes, repair slices, smoke reruns, and later route views without building full orchestration yet

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/cross-track-route-loops`

## Resources
- `task.json`
- `N01-define-route-loop-artifact-model-and-launch-era-plan.md`
- `N01-define-route-loop-artifact-model-and-launch-era-result.md`
- `N02-plan-route-aware-task-inventory-and-apply-next-i-plan.md`
- `N02-plan-route-aware-task-inventory-and-apply-next-i-result.md`
- `N03-decide-when-route-loops-graduate-into-cli-primit-plan.md`
- `N03-decide-when-route-loops-graduate-into-cli-primit-result.md`

## Task Slices
- N01: Define route loop artifact model and launch-era manual workflow. Plan: `N01-define-route-loop-artifact-model-and-launch-era-plan.md`. Result: `N01-define-route-loop-artifact-model-and-launch-era-result.md`.
- N02: Plan route-aware task inventory and apply-next interaction. Plan: `N02-plan-route-aware-task-inventory-and-apply-next-i-plan.md`. Result: `N02-plan-route-aware-task-inventory-and-apply-next-i-result.md`.
- N03: Decide when route loops graduate into CLI primitives. Plan: `N03-decide-when-route-loops-graduate-into-cli-primit-plan.md`. Result: `N03-decide-when-route-loops-graduate-into-cli-primit-result.md`.

## Product Read
DevSpecs now has enough task tracks that release work is no longer a simple linear series. The current v1.1 release path is a route across tracks:

```text
K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03
```

That route means:
- start with a smoke gate
- run targeted repair slices in other tracks
- rerun smoke as an improvement iteration
- only then move to docs and tag work

The immediate need is memory and reconstruction, not autonomous execution. Route loops should preserve intent and ordering so agents do not fall back to "all open tracks are equally next."

## Placeholder Contract
For now, a route loop can live as an authored section in a task index:

```yaml
route:
  id: v1.1-launch-loop
  current: B03
  steps:
    - K01
    - B03
    - H02
    - H03
    - F03
    - K01-1
    - K02
    - K03
  gate:
    promote: advance
    improve: create-or-run-iteration
    block: stop
```

Future CLI support can expose this through inventory and prompt surfaces, but the launch-era version should stay manually authored and inspectable.

## Non-Goals
- Do not build a full orchestration engine in this track.
- Do not make `ds apply next` silently follow a route until route semantics are explicit and user-visible.
- Do not hide normal task status or open-track ambiguity; route loops should clarify priority, not erase state.
- Do not introduce agent runners, tmux orchestration, or vendor-specific execution here.

## Decision Gates
- Promote if a route can be reconstructed after context loss and mapped to concrete task targets.
- Improve if route state exists but does not help `ds task list`/inventory prioritize the next target.
- Rework if the model becomes too close to a hidden workflow engine.
- Block if route loops require unresolved task identity or iteration semantics.

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

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- Prior checkpoint recorded distracting context [low, checkpoint_fact]
  Agent check: Keep that family as reference-only unless this task verifies it.
  Evidence: task v1-1-release-readiness-tag-gate checkpoint cp_20260617T121918Z_k01_validated called distracting `ds task progress on stderr appears as NativeCommandError when PowerShell captures combined streams`

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Pack completeness is not high; verify the working set before editing.

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
Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.

## Suggested Starting Slice
Use `N01-define-route-loop-artifact-model-and-launch-era-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `N01-define-route-loop-artifact-model-and-launch-era-result.md` or `ds task checkpoint`.
