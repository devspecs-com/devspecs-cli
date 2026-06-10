# Task scanless-workflow-ux

## Task
Fold ds scan into workflow commands so onboarding starts with map, find, and task instead of a required manual scan

## Status
packed

## Series
A

## Profile
code-change

## Created At
2026-06-10T13:27:53Z

## Original Query
Fold ds scan into workflow commands so onboarding starts with map, find, and task instead of a required manual scan

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/scanless-workflow-ux`

## Resources
- `task.json`
- `A01-fold-ds-scan-into-workflow-commands-so-onboardin-plan.md`
- `A01-fold-ds-scan-into-workflow-commands-so-onboardin-result.md`

## Task Slices
- A01: Fold ds scan into workflow commands so onboarding starts with map, find, and task instead of a required manual scan. Plan: `A01-fold-ds-scan-into-workflow-commands-so-onboardin-plan.md`. Result: `A01-fold-ds-scan-into-workflow-commands-so-onboardin-result.md`.

## Relevant Map Areas
- `internal/commands`
- `internal/scan`
- `internal/retrieval`
- `internal/evalharness`

## Likely Primary Files
- `internal/commands/task.go` - internal/commands/task.go (go)
  Evidence: anchor-first ranking: score 24.000; matches commands, find, task; fields path, title, body, symbol; query term match in path: commands; query term match in path: task
- `internal/commands/map.go` - internal/commands/map.go (go)
  Evidence: query term match in path: commands; query term match in path: map; query term match in body: fold
- `internal/scan/scan.go` - internal/scan/scan.go (go)
  Evidence: anchor-first ranking: score 24.000; matches scan; fields path, title, symbol, body; query term match in path: scan; query term match in body: into
- `internal/commands/task_evaluate.go` - internal/commands/task_evaluate.go (go)
  Evidence: anchor-first ranking: score 24.000; matches commands, task; fields path, title, body, symbol; query term match in path: commands; query term match in path: task
- `internal/commands/find_pack_companions.go` - internal/commands/find_pack_companions.go (go)
  Evidence: anchor-first ranking: score 24.000; matches scan, commands, find, task; fields body, path, title, symbol; query term match in path: commands; query term match in body: fold
- `internal/commands/scan.go` - internal/commands/scan.go (go)
  Evidence: relationship expansion: source_manifest_family_recovery; query term match in path: commands; query term match in path: scan
- `internal/commands/tldr.go` - internal/commands/tldr.go (go)
  Evidence: query term match in path: commands; query term match in body: fold; query term match in body: instead
- `internal/retrieval/retrieval.go` - internal/retrieval/retrieval.go (go)
  Evidence: query term match in body: commands; query term match in body: fold; query term match in body: into
- `internal/commands/eval.go` - internal/commands/eval.go (go)
  Evidence: query term match in path: commands; query term match in body: instead; query term match in body: map
- `internal/evalharness/eval.go` - internal/evalharness/eval.go (go)
  Evidence: relationship expansion: source_manifest_family_recovery; query term match in body: commands; query term match in body: fold
- `internal/commands/find.go` - internal/commands/find.go (go)
  Evidence: relationship expansion: command_family_companion; pack tier: related (command_family_companion); query term match in path: commands
- `internal/commands/find_pack.go` - internal/commands/find_pack.go (go)
  Evidence: relationship expansion: command_family_companion; pack tier: related (command_family_companion); query term match in path: commands

## Likely Tests
- `internal/commands/map_test.go#L1440` - TestMapAutoScanLeavesUsableIndexForFindPack
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands; query term match in path: map
- `internal/scan/scan_test.go#L248` - TestScan_NewRevisionOnContentChange
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: scan; query term match in body: task
- `internal/commands/task_test.go#L1903` - TestTask_StartUsesGitWorktreeRoot
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands; query term match in path: task
- `internal/commands/eval_test.go#L619` - TestEvalCommand_GraphDiagnosticsRequiresFindCommand
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands
- `internal/commands/find_pack_companions_test.go#L61` - TestAddFindPackCompanionCandidatesAddsCommandFamilyFiles
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands; query term match in body: map
- `internal/commands/find_pack_test.go#L206` - TestWriteFindPackTextFamilyPrimaryVerboseShowsRelatedRows
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands; query term match in body: instead
- `internal/commands/init_test.go#L13` - TestInit_CreatesGlobalDB
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: commands
- `internal/scan/scan_timestamps_test.go#L18` - TestScan_UnchangedBodyLeavesUpdatedAtStable
  Evidence: relationship expansion: same_directory_test_companion; pack tier: related (same_directory_test_companion); query term match in path: scan
- `internal/commands/tldr_test.go#L34` - TestTLDR_FilterAndJSON
  Evidence: relationship expansion: test_companion; pack tier: related (test_companion); query term match in path: commands
- `internal/retrieval/retrieval_test.go#L998` - TestWeightedFilesRetrieverV0_CodeTaskFamilyV2LetsRareAnchorBeatGenericFlow
  Evidence: relationship expansion: test_companion; pack tier: related (test_companion); query term match in title: task
- `internal/commands/retrieval_bridge_test.go#L63` - TestArtifactCandidateIncludesHierarchyMetadataAndLinks
  Evidence: relationship expansion: same_directory_test_companion; pack tier: related (same_directory_test_companion); query term match in path: commands

## Likely Docs / Plans / Config
None found in the initial preflight.

## Supporting Context
None found in the initial preflight.

## Related Git Receipts
- `a50bab8` 2026-05-29 - wip: feat: adapter classifier pipeline (#2)
  Matched paths: `internal/commands/eval.go`, `internal/retrieval/retrieval.go`, `internal/scan/scan.go`
- `701742f` 2026-06-04 - chore: add code task family v2 scout
  Matched paths: `internal/commands/eval.go`, `internal/commands/find_pack_companions.go`, `internal/retrieval/retrieval.go`
- `1b400e6` 2026-06-04 - chore: add code task family ranking scout
  Matched paths: `internal/commands/eval.go`, `internal/commands/find_pack_companions.go`, `internal/retrieval/retrieval.go`

## Noise Risks
None found in the initial preflight.

## Known Knowns
- The preflight found likely primary implementation files.
- The preflight found likely behavior/test artifacts.
- Git receipts provide historical trust evidence for packed paths.

## Known Unknowns
- Pack completeness is not high; verify the working set before editing.

## Confidence Summary
- Primary file confidence: high
- Test coverage confidence: high
- Docs/config coverage confidence: low
- Git receipt confidence: high
- Noise risk: low
- Pack completeness: medium

Why:
- found 12 likely primary file(s)
- found 11 likely test file(s)
- found 3 related Git receipt(s)

Agent instruction:
Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.

## Suggested Starting Slice
Use `A01-fold-ds-scan-into-workflow-commands-so-onboardin-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `A01-fold-ds-scan-into-workflow-commands-so-onboardin-result.md` or `ds task checkpoint`.
