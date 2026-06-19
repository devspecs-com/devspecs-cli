# DevSpecs Public Boundary Inventory

Status: S03/S11 cleanup inventory, 2026-06-09.

This repo is the public product repo for the DevSpecs CLI. It should contain
the CLI, public docs, focused tests, small fixtures, and release assets. Raw
research runs, private feedback notes, broad holdouts, and demo probes should
live in a private research archive unless they have been deliberately
graduated into a small public fixture or public example.

This document began as an inventory pass and now records the public/private
boundary plus completed cleanup actions.

## Repository Boundary

| Repo | Role | Keep Here |
| --- | --- | --- |
| `devspecs-cli` | Public product repo | CLI implementation, install/release config, public docs, focused tests, small deterministic fixtures, public eval smoke tests |
| Private research archive | Private research archive | Research tracks, raw sample logs, holdout construction, eval run archives, demo probes, paper notes, private feedback |
| `devspecs-website` | Public marketing/docs site | Public demo media, public docs, launch pages, contact/telemetry docs |

## Classification Legend

| Classification | Meaning |
| --- | --- |
| `keep-public` | Belongs in the public CLI repo. |
| `keep-public-review` | Probably public, but needs license, attribution, claim, or size review. |
| `summarize-public` | Preserve the learning, but replace raw material with a small public explanation or transcript. |
| `move-private` | Preserve in the private research archive, then remove from the public repo. |
| `archived-private` | Preserved in the private research archive and removed from the public repo. |
| `keep-ignored` | OK as local ignored state; should not be tracked. |
| `delete-after-backup` | Disposable after any paper-relevant or debugging evidence is backed up. |

## Current Public Repo Inventory

| Path | Classification | Rationale | Next Action |
| --- | --- | --- | --- |
| `cmd/`, `internal/` | `keep-public` | Product implementation and focused test coverage. | Keep. |
| `internal/retrieval/pack*`, `internal/commands/task*` | `keep-public` | Core launch behavior for task packing and bounded slice flow. | Keep covered by product tests. |
| `internal/classify/eval*`, `internal/evalharness/`, `internal/commands/eval*` | `keep-public` | Public harness code is acceptable when it is not bundled with private corpora. | Keep, but pair public claims with a small public eval corpus. |
| `README.md`, `LICENSE`, `install.ps1`, `install.sh`, `.goreleaser.yml`, `.github/`, `.githooks/`, `Makefile` | `keep-public` | Product docs, install, release, and development hygiene. | Keep. |
| `TASK_WORKFLOW_EXAMPLE.md` | `keep-public` | Public-safe normalized transcript that replaces raw local task workflow captures. | Keep current with the public CLI UX. |
| `devspecs/tasks/**` | `archived-private` plus `summarize-public` | The CLI repo's own dogfood task workspaces include internal planning, local paths, private strategy, smoke transcripts, and research notes. Product users may still choose to version durable task workspaces in their own repos, but this public repo should not track its internal workspaces. | Preserve privately before removal; keep only deliberately normalized public transcripts such as `TASK_WORKFLOW_EXAMPLE.md`. |
| `fixtures/agentic-saas-fragmented/` | `keep-public-review` | Small deterministic synthetic fixture for local intent-artifact behavior. | Keep if attribution/story text remains synthetic and claim-aligned. |
| `testdata/samples/freetext/`, `testdata/samples/codex/`, `testdata/samples/cursor/`, `testdata/samples/claude/`, `testdata/samples/specify/`, `testdata/samples/false-positives/` | `keep-public` | Narrow parser and retrieval fixtures. | Keep. |
| `testdata/samples/bmad/_bmad-output/**` | `keep-public` | Tiny synthetic BMAD output fixture for format detection. | Keep public; the larger installed BMAD method bundle was removed from the public fixture surface. |
| `.devspecs/raw-output-samples/**` | `archived-private` plus `summarize-public` | Raw CLI UX captures include archives, generated task files, stale behavior, and absolute local paths. | Archived privately and removed from the public repo. Replace only with a short public transcript if needed. |
| `.devspecs/tasks/ds-task-freshness-evaluation-clean/**` | `archived-private` plus `summarize-public` | Dogfood/eval workspace includes local paths and research conclusions that are not product docs. | Archived privately and removed from the public repo. Summarize durable product lessons elsewhere if useful. |
| `.devspecs/eval-runs/**` | `keep-ignored` or `move-private` | Ignored local eval output. | Preserve privately if paper-relevant; otherwise leave ignored or clean locally. |
| `.private/` | `keep-ignored` | Local private notes and scratch material. | Keep ignored. |
| `_ignore/` | `keep-ignored` or `delete-after-backup` | Local scratch area. | Keep ignored; do not bulk-delete without an explicit backup decision. |
| `ds.exe`, `coverage.out`, `.gotmp/`, `.xdg/` | `keep-ignored` or `delete-after-backup` | Local build, coverage, and runtime state. | Keep ignored; regenerate as needed. |

## Cleanup Sequence

Completed on 2026-06-09:

- Copied tracked `.devspecs/raw-output-samples/**` and
  `.devspecs/tasks/ds-task-freshness-evaluation-clean/**` into the private
  research archive with their original paths and source commit.
- Removed those tracked `.devspecs` paths from the public repo.
- Added ignore coverage for raw `.devspecs` task/output folders.
- Added `EVALS.md` for the public eval and fixture boundary.
- Shrunk the BMAD fixture to the synthetic `_bmad-output` files required by
  public adapter tests.
- Added `TASK_WORKFLOW_EXAMPLE.md` as the public-safe replacement for raw task
  workflow samples.

Completed on 2026-06-19:

- Copied tracked `devspecs/tasks/**` into ignored local preservation storage.
- Removed tracked `devspecs/tasks/**` from the public repo.
- Added ignore coverage for this repo's generated `devspecs/tasks/**`
  dogfood workspaces. This is a repo-boundary choice for the public CLI repo,
  not a product rule that forbids users from versioning durable task
  workspaces in their own repositories.

Remaining:

1. Re-run the hygiene checks below before public push.

## Hygiene Checks

Run these checks before public launch and after any cleanup move:

```bash
git ls-files | rg -n "raw|sample|eval|fixture|demo|coverage|private|_ignore|scope|holdout|transcript|vhs|\\.zip|\\.7z|\\.db|\\.exe"
git status --ignored --short
rg -n "C:\\\\Users\\\\[^\\\\]+|C:/Users/[^/]+|/Users/[^/]+|claims\\.zone|dogfood|holdout|ScopeLab|scopelab|raw sample|Focusee|VHS|private" README.md .github cmd internal scripts fixtures testdata .devspecs devspecs -S
```

Expected public posture:

- no tracked local binaries, DBs, raw eval outputs, or coverage files;
- no tracked absolute local paths;
- no private dogfood or research-only conclusions in product docs;
- no tracked internal dogfood task workspaces under `.devspecs/tasks/**` or
  `devspecs/tasks/**`;
- public evals are small, deterministic, and tied to public claims;
- messy research evidence is preserved privately before removal.
