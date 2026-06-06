---
task_id: ds-task-freshness-evaluation-clean
slice: A02
kind: plan
stage: planned
decision: improve
created_at: 2026-06-04T08:51:55Z
---

# A02 Git Worktree Root Detection

## Goal
Fix repo-root detection for Git worktrees so `ds task`, `ds task checkpoint`, and `ds task evaluate` operate against the active checkout.

## Description
The likely bug is in shared repo detection, not in `ds task` itself. `internal/repo/repo.go` currently detects `.git` directories. Git worktrees often have a `.git` file containing a `gitdir:` pointer.

`repo.Detect(worktreeDir)` should treat that checkout directory as the repo root. Task workspaces and evaluation lookup should then naturally land under the active worktree.

## Resources
- `A00-index.md`
- `A02-git-worktree-root-detection-result.md`
- `task.json`
- `internal/repo/repo.go`
- `internal/repo/repo_test.go`
- `internal/commands/refresh.go`
- `internal/commands/task.go`
- `internal/commands/task_evaluate.go`

## Success Criteria
- [ ] `repo.Detect(worktreeDir)` returns `IsGit=true` and `RootPath=worktreeDir` for a Git worktree with a `.git` file.
- [ ] Existing normal `.git` directory tests still pass.
- [ ] `ds task` from a worktree subdirectory writes under that worktree's `.devspecs/tasks`.
- [ ] `ds task checkpoint` and `ds task evaluate` resolve the same workspace path as task creation.

## Tasks
- [ ] Add a `repo.Detect` test for a real `git worktree add` case when Git is available.
- [ ] Add a low-level `.git` file fixture test if the real worktree test is too platform-sensitive.
- [ ] Update repo detection to recognize `.git` files while preserving the checkout directory as root.
- [ ] Confirm remote/branch detection still runs from the worktree root.
- [ ] Add command-level task coverage only if repo-level tests are not enough.
- [ ] Record an implementation checkpoint and update `A02-git-worktree-root-detection-result.md`.

## Decision Gates
- Promote: root detection is fixed in shared repo detection without `ds task` special-casing.
- Improve: task behavior works, but broader repo metadata still needs cleanup.
- Rework: relying on Git CLI is simpler and safer than parsing `.git` files.
- Rollback: the fix destabilizes normal repo detection.
