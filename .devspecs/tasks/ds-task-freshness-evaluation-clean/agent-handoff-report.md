---
task_id: ds-task-freshness-evaluation-clean
stage: completed
decision: promote
created_at: 2026-06-04T12:28:21Z
updated_at: 2026-06-04T12:28:21Z
---

# Agent Handoff Report

## Purpose
This report consolidates what the `ds task` dogfood series learned so a new implementing agent can improve the task/checkpoint/evaluate workflow without overfitting to one Devspecs-on-Devspecs run.

The current track was promoted because the fixes made the workflow safer and easier to evaluate. That does not mean the workflow is done. The remaining issues are mostly context-quality and evaluation-semantics work.

## Current State
- `ds task` emits bounded freshness warnings when likely on-disk anchors may be missing from the indexed candidate pool.
- Git worktree root detection handles `.git` files, not only `.git` directories.
- `ds task evaluate` excludes task workspace artifact reads from metric calculations while preserving raw observed evidence.
- `ds task checkpoint --slice <slice>` can append a checkpoint to the intended slice result.
- Checkpoint markdown lifecycle fields now live in frontmatter.

Verification from the final dogfood evaluation:

```text
usefulness_class: B
primary_file_hit: true
critical_path_recall: 2/5
noise_count: 0
related_commit_surfaced: true
```

Predicted hits:

```text
internal/commands/task.go
internal/commands/task_evaluate.go
```

Misses:

```text
internal/commands/task_test.go
internal/repo/repo.go
internal/repo/repo_test.go
```

The important reading: this is useful enough to start, but still misses companion tests and cross-package support files.

## What Went Wrong

### 1. Stale Index And Retrieval Quality Were Initially Confounded
One failure looked like a ranking/suppression issue, but part of it was probably stale-index behavior. If new task files were not in the index, retrieval could not select them no matter how good the ranking was.

Generalizable lesson:

```text
Before optimizing retrieval suppression, separate stale-index failures from live ranking failures.
```

Implemented response:

```text
Warn when obvious on-disk anchor paths exist but appear absent from indexed candidates.
```

Do not overfit:

```text
The freshness warning is a guardrail, not proof that every miss is caused by index staleness.
```

### 2. Worktree Root Detection Could Point The Workflow At The Wrong Checkout
Git worktrees store `.git` as a file. Root detection previously looked for `.git` directories only, which can cause task workspace paths, repo roots, and evaluation paths to drift.

Generalizable lesson:

```text
Task workflows need boring root/path correctness before higher-level evaluation metrics are trustworthy.
```

Implemented response:

```text
Treat `.git` files and `.git` directories as Git-root anchors.
```

### 3. Evaluation Metrics Were Polluted By Task Workspace Reads
Agents naturally read A00/A01/result/checkpoint files. Those reads are valid observed evidence, but counting them as source-context misses made evaluation look worse and less actionable.

Generalizable lesson:

```text
Observed evidence and metric inputs are related but not identical.
```

Implemented response:

```text
Keep task workspace reads visible in raw observed_context, but exclude task artifacts from hits, misses, recall, companion misses, receipt misses, and confidence mismatch.
```

### 4. Checkpoint UX Made Multi-Slice Work Confusing
Checkpoint markdown had lifecycle fields in body sections, while the user expected frontmatter. `ds task checkpoint` also appended to the first slice by default with no way to target A02/A03/A04.

Generalizable lesson:

```text
Checkpoint metadata should be indexable frontmatter, and multi-slice tasks need explicit slice targeting.
```

Implemented response:

```text
Add checkpoint frontmatter fields and `ds task checkpoint --slice <slice>`.
```

Known caveat:

```text
Do not add `kind: checkpoint` frontmatter yet unless the markdown index kind vocabulary is extended. The current parser rejects unknown artifact kinds.
```

### 5. Companion Test And Support-File Recall Is Still Weak
The final useful pack found the main task command files, but missed the command tests and repo-root detection support files that were actually needed.

Concrete example:

```text
Predicted:
- internal/commands/task.go
- internal/commands/task_evaluate.go

Actually needed:
- internal/commands/task_test.go
- internal/repo/repo.go
- internal/repo/repo_test.go
```

Generalizable lesson:

```text
For implementation tasks, same-package tests and small cross-package support surfaces may matter more than fuzzy topical matches.
```

Do not overfit:

```text
Do not hard-code task.go -> task_test.go or repo.go into task preflight. Improve general companion/test/support-file recall with bounded budget and measurable evals.
```

### 6. Test Companion Recall Metric Is Misleading
The final evaluation reported a companion miss for `internal/commands/task_test.go`, but `test_companion_recall` was `0/0`. That makes the metric hard to interpret.

Generalizable lesson:

```text
If companion misses exist, the companion recall denominator should not quietly look empty.
```

Possible improvement:

```text
Represent companion recall as not-applicable only when no source/test companion relationship was inferred. Otherwise compute an expected companion set or expose a clearer companion_miss_count.
```

## What Not To Conclude From This Run
- Do not conclude that dynamic suppression is the root cause. Another agent is testing that separately.
- Do not conclude that dogfooding should dominate batch statistical evals.
- Do not optimize only for `internal/commands/task*.go`.
- Do not expand packs so aggressively that token usage balloons. The value thesis is grounded improvement without massive token bloat.
- Do not treat a `B` usefulness class as failure. Here it means helpful but incomplete.

## Recommended Next Improvements

### A. Improve Bounded Test Companion Recall
Add a small, budget-aware companion expansion after primary files are selected.

Candidate signals:

```text
same directory `_test.go` files
same package tests near selected command/source files
test files that mention selected function/type/command names
go package-level tests for packages containing selected source files
```

Success criteria:

```text
More implementation tasks include likely tests.
No large fixture/testdata flood.
No significant regression in statistical eval corpora.
Token budget stays bounded.
```

### B. Improve Cross-Package Support-File Recall
When a selected file calls into a small local support package, task preflight should consider a bounded expansion to that support surface.

Example from this run:

```text
internal/commands/task.go needed internal/repo/repo.go and internal/repo/repo_test.go for root detection behavior.
```

Possible signals:

```text
local imports from selected files
recent edits/checkpoints mentioning support package paths
same change-set receipts touching selected and support packages
```

### C. Clean Up Companion Metrics
Make `test_companion_recall` interpretable.

Recommended shape:

```text
expected_test_companions: N
hit_test_companions: M
missed_test_companions: [...]
recall: M/N or n/a
```

If the system cannot infer expected companions, say so explicitly instead of returning a misleading `0/0` next to companion misses.

### D. Use Checkpoints As Intent And Progress Substrate
Checkpoint JSON is now structured enough to use as future retrieval evidence. The next pass should explore a small, bounded indexing strategy.

Useful checkpoint signals:

```text
stage
decision
slice
files_read
files_edited
tests_run
missed_files
noise_files
next_recommended_slice
```

Guardrail:

```text
Index checkpoint summaries and path signals, not whole verbose checkpoint bodies by default.
```

### E. Keep Dogfood And Eval Corpora Separate
The next agent should evaluate changes in at least three buckets:

```text
1. This dogfood task series
2. A small one-off batch of Devspecs tasks
3. Existing statistical eval corpus or ablation harness
```

The comparison should report both lift and regressions. Avoid letting a single dogfood case dominate default retrieval behavior.

## Suggested Prompt For The Next Agent

```text
Start from `.devspecs/tasks/ds-task-freshness-evaluation-clean/agent-handoff-report.md`.

Please improve `ds task` context quality without overbuilding. Focus on bounded test companion recall, cross-package support-file recall, and clearer companion metrics. Do not change dynamic suppression unless needed for this task; another agent is testing that in parallel.

Use the current `ds task` workflow end to end:
- read A00 and this handoff report;
- create or update a task slice if useful;
- implement one bounded change;
- checkpoint with structured JSON and `--slice`;
- run `ds task evaluate ds-task-freshness-evaluation-clean --json`;
- compare dogfood results against broader eval or ablation evidence;
- report friction points and any regressions.

Be careful about token bloat. Prefer small companion expansions with clear evidence over broad pack inflation.
```

## Overlap Warning For Parallel Work
Dynamic suppression, default test inclusion, and retrieval ranking may overlap with `internal/retrieval` and evaluation harness files. If the next change touches those areas while another agent is active, pause and inspect git status before editing.

The safest near-term implementation surfaces are:

```text
internal/commands/task.go
internal/commands/task_evaluate.go
internal/commands/task_test.go
```

The likely overlap surfaces are:

```text
internal/retrieval/*
internal/evalharness/*
```

## Final Recommendation
Promote the current task/checkpoint/evaluate workflow as experimental, then improve context quality in small, measured slices. The strongest next bet is not more planning UI. It is bounded companion/support-file recall plus cleaner metrics, validated against both dogfood and broader eval evidence.
