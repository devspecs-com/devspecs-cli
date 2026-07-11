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

## Activation Matrix Goldens

`ds eval --activation-matrix` is a hidden developer harness for fast-activation
work on `ds recent` and `ds map`. It runs a YAML or JSON manifest of repos and
commands, forces `--json --quiet --path <repo>`, normalizes local repo paths out
of stdout, and compares result output against per-profile goldens.

## Canonical Regression Set Registry

Local/private activation work keeps a discoverability registry at
`.devspecs/eval-runs/_registry/REGRESSION_SETS.md`, with machine-readable set
metadata in `.devspecs/eval-runs/_registry/manifest-registry.yaml`.

Search anchors for future agents:

- `smoke-5-recent-quality`: current 5-repo `ds recent` quality gate.
- `skinny-25-recent-legacy`: old local 25-repo recent stability gate.
- `fat-full-fastapi-quality-and-scan`: current FastAPI full-history sentinel;
  one repo only, not broad fat evidence.
- `fat-100-vps-daily-target`: target shape for a durable broad daily gate.
- `fat-156-recent-legacy`: H03 historical recent fat gate, not current quality
  proof.
- `fat-100-map-full-checkout-historical`: M18 validation100 map audit in
  `devspecs-sample-miner`, full-checkout evidence but failed handoff promotion.

The `.devspecs/eval-runs/` registry is intentionally local/private and may be
absent from clean public checkouts. When present, validate it with:

```bash
go test ./internal/commands -run TestEvalRegressionSetRegistryIfPresent -count=1
```

Current-snapshot goldens are forward regression guards only. Promotion-quality
optimization evidence must compare the candidate against a pre-optimization
baseline on the same locked repo set. Faster-but-weaker default activation
output is a regression, not a successful fast path.

Example manifest:

```yaml
version: 1
repos:
  - id: skinny-example
    path: ../some-small-oss-repo
    profiles: [skinny]
    commands:
      - name: recent
        args: ["auth", "--max-areas", "5"]
      - name: map
        args: ["--max-areas", "5"]
  - id: fat-example
    path: C:/repos/some-large-oss-repo
    profiles: [fat]
    commands:
      - name: recent
        args: ["payment", "--max-areas", "8"]
      - name: map
        args: ["--max-areas", "8"]
```

Create or refresh the current HEAD snapshot:

```bash
ds eval activation-matrix.yaml --activation-matrix --activation-profile skinny --activation-update --json
```

Compare later runs against the snapshot:

```bash
ds eval activation-matrix.yaml --activation-matrix --activation-profile skinny --json
ds eval activation-matrix.yaml --activation-matrix --activation-profile fat --json
```

Compare a baseline binary against a candidate binary:

```bash
ds eval activation-matrix.yaml \
  --activation-matrix \
  --activation-profile skinny \
  --activation-clone-mode full \
  --activation-baseline-bin /tmp/ds-baseline \
  --activation-candidate-bin ./ds \
  --activation-result-dir .devspecs/eval-runs/h04-2/results \
  --json
```

Add `--activation-map-structured` only when auditing `ds map` action quality
rather than raw bytes. The structured gate still fails changed area labels,
classes, roles, evidence counts, key paths, `try` commands, or caveats. It can
accept boundary path, raw-anchor, trace-receipt, trace-term, and support-count
diagnostic drift plus removal of the stale missing-index caveat, and records
those decisions in `map_action_quality`.

Compare default cold first-run output against warm-index output when auditing
whether an optimization weakens first interaction quality:

```bash
ds eval activation-matrix.yaml \
  --activation-matrix \
  --activation-profile skinny \
  --activation-clone-mode full \
  --activation-baseline-bin /tmp/ds-baseline \
  --activation-candidate-bin ./ds \
  --activation-baseline-index-state cold \
  --activation-candidate-index-state warm \
  --activation-result-dir .devspecs/eval-runs/h06/cold-vs-warm \
  --json
```

By default the harness forces `--json --quiet --path <repo>` for each command.
Use `--activation-quiet=false` only when comparing against a baseline binary
that does not support `--quiet`; stderr is then recorded as evidence instead of
being treated as quiet-output drift.

Result JSON records the manifest schema, normalization version, suite/profile,
selected clone mode, binary IDs, repo-set metadata, per-case durations, and
aggregate p50/p95/max timings. When `--activation-result-dir` is set, the same
result is written to `activation-matrix-result.json`; binary comparison mode
also writes captured stdout/stderr under `outputs/baseline/` and
`outputs/candidate/`. Result JSON also records `index_state`,
`baseline_index_state`, `candidate_index_state`, and warmup sizes/timing for
warm binary-comparison runs. Cold is the default because the first command on a
fresh local install is the activation bar; warm comparisons are an audit tool,
not a product promise that weaker cold output is acceptable.

Keep raw manifests and generated goldens in ignored eval-run storage unless a
public-safe fixture is deliberately derived. Optimization subiterations should
run the skinny gate while iterating and run the fat gate before promotion when
the representative set is available.

The canonical skinny set should use full-history clones. Fat should prefer full
commit history; blobless partial clones are acceptable when the commit graph is
complete. Shallow clones are only for explicit ad hoc smoke mode. A future full
profile should be runnable on VPS/daily infrastructure with full clone
materialization.

## Activation Scan Benchmark

`ds eval --activation-scan-benchmark` is the hidden scan/indexing performance
side of activation QA. It reuses the activation manifest format, selects repos
by profile, runs only `scan` commands from each repo entry, and defaults to one
plain `scan` command when a repo has no `scan` command. The runner forces
`--json --quiet --phase-timing --path <repo>` and records reduced diagnostics
instead of comparing raw scan stdout, because scan JSON intentionally contains
timing and generated IDs.

The result file is `activation-scan-benchmark-result.json`. It records:

- wall time p50/p95/max for scan cases;
- aggregate p50/p95/max by scan phase;
- DB size including WAL/SHM sidecar files;
- row counts by table and table group;
- source manifest file/symbol/test/import/FTS counts;
- traversal inventory and skipped-directory counts;
- retained scan stdout/stderr under `outputs/scan/`;
- optional regression failures against a previous benchmark result.

Example cold fat/full benchmark:

```bash
ds eval .devspecs/eval-runs/c03/c03-fat-full-scan-benchmark.yaml \
  --activation-scan-benchmark \
  --activation-profile fat \
  --activation-clone-mode full \
  --activation-index-state cold \
  --activation-result-dir .devspecs/eval-runs/c03/daily-fat-scan-cold \
  --json
```

Example default-then-task-substrate upgrade benchmark:

```bash
ds eval .devspecs/eval-runs/c03/c03-fat-full-scan-benchmark.yaml \
  --activation-scan-benchmark \
  --activation-profile fat \
  --activation-clone-mode full \
  --activation-index-state warm \
  --activation-scan-baseline-result .devspecs/eval-runs/c03/baseline-warm/activation-scan-benchmark-result.json \
  --activation-scan-max-regression-ratio 1.30 \
  --activation-scan-max-regression-ms 5000 \
  --activation-result-dir .devspecs/eval-runs/c03/daily-fat-scan-warm \
  --json
```

Daily/VPS activation regression should run both pieces:

1. Quality preservation: run `--activation-matrix` in binary comparison mode
   for `recent` and `map` on locked full-history repos. This is the first-run
   output gate. Timing-only success is not sufficient.
2. Indexing performance: run `--activation-scan-benchmark` on the fat/full scan
   manifest in cold mode and, where relevant, warm/default-then-upgrade mode.
   Compare to a checked or retained benchmark result with relative thresholds
   and a fixed millisecond slack.

Default hosted runners are acceptable for smoke checks and skinny output gates.
Use a controlled VPS or self-hosted runner for fat/full performance thresholds:
pin clone SHAs, keep full commit history, isolate `DEVSPECS_HOME`, upload raw
artifacts, and evaluate rolling medians rather than one-off hosted-run timing.

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
