# Task 20260616-132238-fix-workspace-root-warning-so-normal-git-repo-ro A01 Plan

## Goal
Suppress workspace-root warning for normal git repo roots.

## Context
Current behavior warns whenever a root has two child directories with project markers. That catches workspace parents, but also catches real app repos with a top-level `.git` and child packages.

## Expected Change
- Treat a top-level `.git` root as intentionally selected unless child candidates are nested Git repos.
- Keep warning for workspace parents that contain multiple child projects/repos and no top-level `.git`.
- Keep warning for roots that contain multiple nested Git repos.

## Success Criteria
- [ ] Direct heuristic test covers `.git` root plus `api`, `web`, `signer` child package manifests with no warning.
- [ ] Existing workspace-parent warning tests still pass.
- [ ] Map/scan JSON warning behavior remains stderr-only when a warning is emitted.

## Decision Gate
- Promote: false-positive warning is gone and workspace-parent warnings remain covered.
- Improve: warning is less noisy but still ambiguous in common app repos.
- Rework: actual workspace-parent warnings are lost.
