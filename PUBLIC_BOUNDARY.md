# DevSpecs Public Boundary Inventory

Status: S03/S11 cleanup inventory, 2026-06-09.

This repo is the public product repo for the DevSpecs CLI. It should contain
the CLI, public docs, focused tests, small fixtures, and release assets. Raw
research runs, private dogfood notes, broad holdouts, and demo probes should
live in the private research archive unless they have been deliberately
graduated into a small public fixture or public example.

No files were moved or deleted during this inventory pass. The goal is to make
the next cleanup slice mechanical and reversible.

## Repository Boundary

| Repo | Role | Keep Here |
| --- | --- | --- |
| `devspecs-cli` | Public product repo | CLI implementation, install/release config, public docs, focused tests, small deterministic fixtures, public eval smoke tests |
| `devspecs-sample-miner` | Private research archive | Research tracks, raw sample logs, holdout construction, eval run archives, demo probes, paper notes, private dogfood feedback |
| `devspecs-website` | Public marketing/docs site | Public demo media, public docs, launch pages, contact/telemetry docs |

## Classification Legend

| Classification | Meaning |
| --- | --- |
| `keep-public` | Belongs in the public CLI repo. |
| `keep-public-review` | Probably public, but needs license, attribution, claim, or size review. |
| `summarize-public` | Preserve the learning, but replace raw material with a small public explanation or transcript. |
| `move-private` | Preserve in the private research archive, then remove from the public repo. |
| `keep-ignored` | OK as local ignored state; should not be tracked. |
| `delete-after-backup` | Disposable after any paper-relevant or debugging evidence is backed up. |

## Current Public Repo Inventory

| Path | Classification | Rationale | Next Action |
| --- | --- | --- | --- |
| `cmd/`, `internal/` | `keep-public` | Product implementation and focused test coverage. | Keep. |
| `internal/retrieval/pack*`, `internal/commands/task*` | `keep-public` | Core launch behavior for task packing and bounded slice flow. | Keep covered by product tests. |
| `internal/classify/eval*`, `internal/evalharness/`, `internal/commands/eval*` | `keep-public` | Public harness code is acceptable when it is not bundled with private corpora. | Keep, but pair public claims with a small public eval corpus. |
| `README.md`, `LICENSE`, `install.ps1`, `install.sh`, `.goreleaser.yml`, `.github/`, `.githooks/`, `Makefile` | `keep-public` | Product docs, install, release, and development hygiene. | Keep. |
| `fixtures/agentic-saas-fragmented/` | `keep-public-review` | Small deterministic synthetic fixture for local intent-artifact behavior. | Keep if attribution/story text remains synthetic and claim-aligned. |
| `testdata/samples/freetext/`, `testdata/samples/codex/`, `testdata/samples/cursor/`, `testdata/samples/claude/`, `testdata/samples/specify/`, `testdata/samples/false-positives/` | `keep-public` | Narrow parser and retrieval fixtures. | Keep. |
| `testdata/samples/bmad/**` | `keep-public-review` | Larger third-party-shaped sample tree; useful coverage, but it deserves license and footprint review before launch. | Keep only if license/attribution is clean; otherwise replace with a smaller synthetic fixture. |
| `.devspecs/raw-output-samples/**` | `move-private` plus `summarize-public` | Tracked raw CLI UX captures include archives, generated task files, stale behavior, and absolute local paths. | Preserve in private research archive, remove from public repo, and replace only with a short public transcript if needed. |
| `.devspecs/tasks/ds-task-freshness-evaluation-clean/**` | `move-private` plus `summarize-public` | Tracked dogfood/eval workspace includes local paths and research conclusions that are not product docs. | Preserve privately, remove from public repo, and summarize durable product lessons elsewhere if useful. |
| `.devspecs/eval-runs/**` | `keep-ignored` or `move-private` | Ignored local eval output. | Preserve privately if paper-relevant; otherwise leave ignored or clean locally. |
| `.private/` | `keep-ignored` | Local private notes and scratch material. | Keep ignored. |
| `_ignore/` | `keep-ignored` or `delete-after-backup` | Local scratch area. | Keep ignored; do not bulk-delete without an explicit backup decision. |
| `ds.exe`, `coverage.out`, `.gotmp/`, `.xdg/` | `keep-ignored` or `delete-after-backup` | Local build, coverage, and runtime state. | Keep ignored; regenerate as needed. |
| `testdata/samples/bmad/_bmad/custom/config.user.toml` | `keep-ignored` | Local user config under a public fixture tree. | Keep ignored and ensure it never becomes tracked. |

## Cleanup Sequence

1. Copy tracked `.devspecs/raw-output-samples/**` and
   `.devspecs/tasks/ds-task-freshness-evaluation-clean/**` into the private
   research archive with their original paths and commit references.
2. Remove those tracked `.devspecs` paths from the public repo after the private
   archive copy is confirmed.
3. Add ignore coverage for raw `.devspecs` task/output folders that should never
   be committed by accident.
4. Replace raw samples with a small public transcript or example only if it is
   generated from current public CLI commands and contains no local paths.
5. Add a public `EVALS.md` that explains the small public eval corpus and states
   that private research runs are separate.
6. Review `testdata/samples/bmad/**` for license, attribution, and footprint;
   shrink or replace it if the public value is not worth the surface area.
7. Re-run the hygiene checks below before public push.

## Hygiene Checks

Run these checks before public launch and after any cleanup move:

```bash
git ls-files | rg -n "raw|sample|eval|fixture|demo|coverage|private|_ignore|scope|holdout|transcript|vhs|\\.zip|\\.7z|\\.db|\\.exe"
git status --ignored --short
rg -n "C:\\\\Users|brenn|devspecs-sample-miner|claims\\.zone|dogfood|holdout|ScopeLab|scopelab|raw sample|Focusee|VHS|private" README.md .github cmd internal scripts fixtures testdata .devspecs -S
```

Expected public posture:

- no tracked local binaries, DBs, raw eval outputs, or coverage files;
- no tracked absolute local paths;
- no private dogfood or research-only conclusions in product docs;
- public evals are small, deterministic, and tied to public claims;
- messy research evidence is preserved privately before removal.
