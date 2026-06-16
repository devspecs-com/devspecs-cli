# Task 20260616-132238-fix-workspace-root-warning-so-normal-git-repo-ro

## Goal
Fix workspace root warning so normal Git repo roots with child app folders do not always look like mistaken workspace roots.

## Product Signal
Dogfood showed the warning appearing in a real repo root with `.git` at the top and child projects such as `api`, `web`, and `signer`. That makes DevSpecs sound unsure even when the user is already at the intended repo root.

## Slice
- A01: Suppress workspace-root warning for normal git repo roots. Plan: `A01-fix-workspace-root-warning-plan.md`. Result: `A01-fix-workspace-root-warning-result.md`.

## Decision Gate
- Promote if repo roots with top-level `.git` and ordinary child app manifests no longer warn.
- Preserve if workspace parents with multiple child repos still warn.
- Rework if the warning disappears for actual workspace-parent mistakes.
