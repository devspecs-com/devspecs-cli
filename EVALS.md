# DevSpecs Public Eval Surface

Status: public eval boundary, 2026-06-09.

This repo includes the public, claim-aligned test and fixture surface for the
DevSpecs CLI. It is meant to prove deterministic product behavior and guard
public claims. It is not a dump of exploratory research material or unreduced
evaluation runs.

## What The Public Eval Surface Covers

- discovery of common intent-artifact layouts;
- parser and metadata extraction behavior;
- task lifecycle and bounded slice prompt behavior;
- retrieval and packing contracts that are stable enough for product tests;
- small synthetic fixtures that can run quickly in CI.

The default public check is:

```bash
go test -count=1 ./...
```

Useful narrower checks:

```bash
go test -count=1 ./internal/adapters/markdown
go test -count=1 ./internal/commands
go test -count=1 ./internal/retrieval
go test -count=1 ./internal/evalharness
```

## Public Fixtures

| Path | Role | Public Posture |
| --- | --- | --- |
| `fixtures/agentic-saas-fragmented/` | Synthetic fragmented SaaS repo with plans, ADRs, PRDs, OpenSpec changes, and code anchors. | Keep public, but continue to keep it synthetic and claim-aligned. |
| `testdata/samples/freetext/` | Small free-text planning and roadmap samples. | Keep public. |
| `testdata/samples/codex/`, `testdata/samples/cursor/`, `testdata/samples/claude/` | Minimal agent-specific plan layout samples. | Keep public. |
| `testdata/samples/specify/` | Synthetic Spec Kit-style layout. | Keep public. |
| `testdata/samples/false-positives/` | Negative examples for parser noise. | Keep public. |
| `testdata/samples/bmad/_bmad-output/` | Synthetic BMAD output layout for format detection. | Keep public. Do not ship the full installed BMAD method bundle as a fixture. |
| `TASK_WORKFLOW_EXAMPLE.md` | Public-safe normalized transcript from current `ds task` commands against a tiny synthetic repo. | Keep current with launch UX; do not replace it with raw local demo captures. |

## What Stays Private

The public repo should not include:

- full private holdout corpora;
- raw research tracks or failed ranking experiments;
- broad scout scripts and mined repository bundles;
- raw dogfood feedback or private project notes;
- raw demo capture folders and replay scripts;
- local `.devspecs` task workspaces, raw output samples, DBs, or eval runs.

Those should stay outside the public repo until a narrow, current, public-safe
fixture or transcript is intentionally derived from them.

## Claim Boundary

Public tests show that the CLI behavior is deterministic, documented, and
regression-tested. They do not prove broad retrieval superiority or publish the
full research runway.

Public demos should be described honestly:

- live command demos should use commands that are available in the public CLI;
- captured demos should say they are captured;
- generated transcripts should contain no local paths, private repo names,
  secrets, or stale behavior from older CLI builds.
- public examples should point to [`TASK_WORKFLOW_EXAMPLE.md`](TASK_WORKFLOW_EXAMPLE.md)
  when demonstrating bounded task/slice lifecycle behavior.

## Fixture Admission Rules

New public fixtures should be:

- synthetic or clearly license-safe;
- small enough for routine CI;
- tied to a public product claim;
- free of local absolute paths, secrets, private repo names, and raw user data;
- documented with why the fixture exists and what tests depend on it.

If a fixture is large, copied from a third-party project, or only useful for
research exploration, keep it private and graduate a smaller synthetic fixture
instead.
