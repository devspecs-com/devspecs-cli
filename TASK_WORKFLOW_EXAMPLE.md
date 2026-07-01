# DevSpecs Task Workflow Example

Status: public-safe transcript, generated from the current CLI on 2026-06-09.

This example was captured from a tiny synthetic repo with:

- `docs/plans/weekly-digest.md`
- `services/notifications/digest.go`
- `services/notifications/digest_test.go`

The commands are real DevSpecs CLI commands. Local absolute path prefixes were
normalized to `<repo>` and long output is shortened only where marked.

Before handing work to an agent, run `ds tldr`. It gives the agent-facing rules:
one bounded target, read the relevant artifacts, checkpoint actual evidence, and
stop at the decision gate.

If the repo was initialized with agent adapters, the shortest equivalent path is
usually `/ds-task "goal"` followed by `/ds-apply <task-id>` for the next slice.
Those adapters are thin wrappers over the same CLI commands shown below.

This example starts with `ds task` because the work item is already known. In a
brownfield repo where the target is unclear, start with `ds recent`, then use
`ds find` or `ds map` as trust/evidence checks. Switch to `ds task` once the execution
target is concrete. Diagnostics are not prerequisites for known work.

For an umbrella workspace with multiple child repos, keep repo execution
explicit. Workspace coordination creates the shared change record, but repo-local
task work still uses `--repo <child-repo>` so task artifacts are not written into
the umbrella root by accident.

DevSpecs does not replace canonical repo plans. Existing `PLAN-*` files, ADRs,
PRDs, RFCs, decision memos, and runbooks remain the source of truth. A task
workspace is the execution layer on top: it turns that intent into addressable
slices and records what happened.

## Create A Bounded Task

Use full `ds task` for multi-slice work where handoff and receipts matter. For a
small bugfix or doc spike, `ds task "goal" --quick` is usually enough.

```bash
$ ds task "Add a weekly digest email for unread notifications" \
  --id weekly-digest \
  --slice "Trace existing digest behavior and tests" \
  --slice "Add weekly digest scheduling contract"
```

```text
Created task workspace: <repo>/devspecs/tasks/weekly-digest
Task ID: weekly-digest
Series: A
Profile: code-change
A00: <repo>/devspecs/tasks/weekly-digest/A00-index.md
A01 plan: <repo>/devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-plan.md
A01 result: <repo>/devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-result.md
A02 plan: <repo>/devspecs/tasks/weekly-digest/A02-add-weekly-digest-scheduling-contract-plan.md
A02 result: <repo>/devspecs/tasks/weekly-digest/A02-add-weekly-digest-scheduling-contract-result.md
Confidence: primary=medium tests=high completeness=low noise=low
Indexed: devspecs/tasks/weekly-digest/A00-index.md, devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-plan.md, devspecs/tasks/weekly-digest/A02-add-weekly-digest-scheduling-contract-plan.md
Task index updated (4 new, 0 updated)
```

The generated `A00` index captured source, test, planning, and git-receipt
context:

```text
## Likely Primary Files
- `services/notifications/digest.go`
  Evidence: query term match in path: digest; query term match in path: notifications; query term match in body: unread

## Likely Tests
- `services/notifications/digest_test.go`
  Evidence: query term match in path: digest; query term match in path: notifications; query term match in body: unread
- `services/notifications/digest_test.go#L12` - TestBuildWeeklyDigestIncludesWorkspaceSubject
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: digest; query term match in path: notifications
- `services/notifications/digest_test.go#L5` - TestBuildWeeklyDigestSkipsEmptyDigest
  Evidence: relationship expansion: source_manifest_loss_safe_preserved; query term match in path: digest; query term match in path: notifications

## Likely Docs / Plans / Config
- `docs/plans/weekly-digest.md` - Weekly Digest
  Evidence: indexed section match: Goal lines 3-6; query term match in path: digest; query term match in path: weekly

## Related Git Receipts
- `4044bb9` 2026-06-09 - Seed notification digest example
  Matched paths: `docs/plans/weekly-digest.md`, `services/notifications/digest.go`, `services/notifications/digest_test.go`

## Confidence Summary
- Primary file confidence: medium
- Test coverage confidence: high
- Docs/config coverage confidence: medium
- Git receipt confidence: medium
- Noise risk: low
- Pack completeness: low
```

## Address One Slice

```bash
$ ds task show A01
```

```text
Task target: A01
Task ID: weekly-digest
Series: A
Profile: code-change
Title: Trace existing digest behavior and tests
Stage: -
Decision: -
Plan: <repo>/devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-plan.md
Result: <repo>/devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-result.md
Out-of-scope sibling targets: A02

Plan body:
# Task weekly-digest A01 Plan

## Goal
Trace existing digest behavior and tests

## Resources
- `A00-index.md`
- `A01-trace-existing-digest-behavior-and-tests-result.md`
- `task.json`
- `services/notifications/digest.go`
- `services/notifications/digest_test.go`
- `services/notifications/digest_test.go#L12`
- `services/notifications/digest_test.go#L5`
- `docs/plans/weekly-digest.md`

## Starting Context
### Files to Inspect First
- `services/notifications/digest.go`

### Tests to Inspect First
- `services/notifications/digest_test.go`
- `services/notifications/digest_test.go#L12`
- `services/notifications/digest_test.go#L5`

## Expected Change Surface
- `services/notifications/digest.go`

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Treating the generated context as complete without verification.
```

## Emit A Bounded Agent Prompt

```bash
$ ds apply weekly-digest
```

````text
You are working on DevSpecs task weekly-digest target A01 only.

Boundary:
```yaml
devspecs:
  task_id: weekly-digest
  target: A01
  allowed_scope: slice
  plan: devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-plan.md
  result: devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-result.md
  must_not_implement:
    - A02
```

Goal: Trace existing digest behavior and tests

Do not implement sibling slices, future slices, or the full task track. Stop after this target's acceptance checks are satisfied.
Record the outcome in `devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-result.md` or with `ds task checkpoint weekly-digest --target A01`.
Checklist edits are useful notes, but lifecycle state should be recorded with `ds task checkpoint`; legacy `finish` and `decide` shortcuts are compatibility-only.
Command roles: use `ds find` to discover and pack evidence, `ds task status`
to inspect lifecycle, `ds apply` to emit the current bounded prompt, and
`ds workspace trace` only for known workspace change/task links. In trace
output, `status` and `index_status` are separate signals.
At the end, recommend exactly one decision: promote, improve, rework, rollback, or block.
Also answer the completion contract: attempted slice, gate tested, what changed,
evidence for the decision, what remains, and the next iteration.
````

## Record The Decision Gate

This example uses `--index=false` on lifecycle writes only to keep the transcript
compact. Omitting it also recaptures the updated task artifacts into the local
DevSpecs index.

```bash
$ ds task start A01 --index=false
$ ds task checkpoint weekly-digest \
  --target A01 \
  --stage validated \
  --decision promote \
  --description "Verified the existing digest builder and focused tests before scheduling work." \
  --file-read services/notifications/digest.go \
  --test-read services/notifications/digest_test.go \
  --test-run "go test ./services/notifications" \
  --learning "test_surface|Digest behavior is covered by same-package Go tests.|high|weekly-digest|services/notifications/digest_test.go" \
  --next-target A02 \
  --next-decision promote \
  --index=false
```

```text
Updated A01: stage=started decision=continue
Manifest: <repo>/devspecs/tasks/weekly-digest/task.json
Index: <repo>/devspecs/tasks/weekly-digest/A00-index.md
Recorded checkpoint: <repo>/devspecs/tasks/weekly-digest/checkpoints/20260609-134754-validated.md
Structured checkpoint: <repo>/devspecs/tasks/weekly-digest/checkpoints/20260609-134754-validated.json
Updated result: <repo>/devspecs/tasks/weekly-digest/A01-trace-existing-digest-behavior-and-tests-result.md
```

```bash
$ ds task status weekly-digest
```

```text
Task ID: weekly-digest
Series: A
Profile: code-change
Status: packed
Updated At: 2026-06-09T13:47:54Z
Next: A02 - Add weekly digest scheduling contract
Run: ds apply weekly-digest
A01: Trace existing digest behavior and tests [slice] stage=validated decision=promote checkpoint=checkpoints/20260609-134754-validated.md checkpoint_id=cp_20260609T134754Z_a01_validated
A02: Add weekly digest scheduling contract [slice]
```

`ds task status` answers lifecycle questions: which slice is started,
validated, promoted, blocked, or next. `ds apply` emits the bounded prompt for
that target. Use `ds find` when you need to discover source/docs/tests for a
question. Use `ds workspace trace` only when you already know a workspace change
or repo task ID and need linked repo slices.

## What This Shows

- `ds task` creates addressable task and slice artifacts.
- The generated task index carries source, test, doc, and git-receipt evidence.
- `ds apply` gives an agent a one-slice boundary instead of the whole task track.
- `ds task checkpoint` records the actual evidence and decision gate.
- `ds task status` shows the next slice before another agent prompt is emitted.

This is a small synthetic example. It is not a broad retrieval benchmark. In a
real brownfield repo, use `ds recent`, `ds find`, and `ds map` to route to the
current owner decision docs, then use `devspecs/tasks/*` for bounded execution
and receipts.

## Experimental Workspace Coordination

For multi-repo dogfood, initialize the umbrella and create repo-local task
slices explicitly:

```powershell
ds workspace init . --json
ds workspace change create "Customer export across frontend/backend" --workspace . --repos backend,frontend,database,prefect --json
ds workspace slice create EAG-C001 --workspace . --repo backend --name "Backend API" --json
ds task show eag-c001-backend --repo ./enalytics-backend --json
ds apply eag-c001-backend --repo ./enalytics-backend --json
ds workspace trace EAG-C001 --workspace . --json
```

This surface is intentionally separate from the single-repo task workflow. Use
`ds recent` or `ds find` to discover context; use `ds workspace trace` only when
you already know the workspace change or repo task ID.
Trace lifecycle `status` and index-capture `index_status` are separate signals.
