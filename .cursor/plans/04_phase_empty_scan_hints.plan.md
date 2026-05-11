---
name: Phase 4 — empty scan hints
overview: When configured scan finds zero artifacts, print bounded non-interactive hints using discover heuristics + ds config examples; exit 0.
todos:
  - id: p4-zero-branch
    content: Branch in runScan when total artifacts found == 0 (all adapters).
    status: completed
  - id: p4-bounded-discover
    content: Call discover with strict caps and ignore rules; exclude ignored paths from candidates.
    status: completed
  - id: p4-human-hints
    content: "Human output: message + capped candidates + ds config add-source example; exit 0."
    status: completed
  - id: p4-json-hints
    content: "ds scan --json: hints array shape; document; empty when not applicable."
    status: completed
  - id: p4-quiet
    content: Define and test --quiet behavior (hints suppressed vs exit 0).
    status: completed
  - id: p4-tests
    content: "Tests: exit 0, bounded output, ignored path excluded, non-zero scan unchanged, JSON shape."
    status: completed
  - id: p4-commits
    content: Incremental commits per index discipline.
    status: completed
isProject: false
---

# Phase 4: Empty scan hints

**Goal**: Recovery path when config points at nothing useful — **no prompts**, “modern CLI” hints only.

## Tasks

- [x] **Zero detection**: in [`runScan`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/scan.go) (or scanner result), branch when **total artifacts found == 0** (all adapters).
- [x] **Bounded discover**: invoke `internal/discover` (or shared helper) with strict limits; reuse ignore matcher; **exclude** ignored paths from candidates.
- [x] **Human output**: print “No artifacts…” + capped candidate list + at least one `ds config add-source markdown <path>` example; **exit 0**.
- [x] **`--json`**: add **`hints`** array (path + optional `suggest_command` per entry); document shape; when there are no on-disk candidates the **`hints`** key is **omitted** (`encoding/json` **`omitempty`** on an empty slice), not emitted as `"hints": []`. When the scan found hits, **`hints`** is absent as well.
- [x] **`--quiet`**: define behavior (suppress hints vs still exit 0 — document and test).
- [x] **Tests**: zero-artifact fixture (exit 0, bounded lines); ignored path not in hints; non-zero scan unchanged; JSON shape test.
- [x] **Commits**: incremental per index discipline.

## Behavior

After [`scanner.Run`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go), if **total indexed/ found count is zero** (all adapters):

1. Run **bounded** discovery pass (reuse `internal/discover` from phase 3 — cap cost, respect ignores).
2. Print short message, e.g.:

```text
No artifacts found in configured paths.

Possible candidates:
  docs/design/
  .cursor/plans/
  specs/001-auth/

Add one:
  ds config add-source markdown docs/design
```

3. Exit **0** (successful no-op scan, not an error).

## Constraints

- **Never** interactive; works in CI.
- Do not spam unbounded paths — cap list (e.g. top N by confidence).
- **Never** suggest ignored paths.

## Touchpoints

- [`internal/commands/scan.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/scan.go)
- `internal/discover` (phase 3)

## Auditable success criteria (phase 4)

- [x] Integration or command test: configured repo with **empty** source paths (or paths with no matching files) yields **exit code 0** from `ds scan`.
- [x] Stdout (or stderr per product choice) contains a **bounded** candidate list: **≤ N** paths or lines (N fixed in test).
- [x] At least one hint line suggests **`ds config add-source`** (or equivalent) with a **concrete path** fragment.
- [x] Fixture with ignored candidate path: that path **never** appears in hints (reuse ignore matcher).
- [x] **`ds scan --json`** on zero-artifact run: JSON parses; **`hints`** key is present when at least one candidate exists, **omitted** when there are no candidates (same as non-zero scan); no prompt fields.
- [x] Non-zero-artifact scan: **no** hint block (or hints empty/absent per spec) — regression test.
- [x] Incremental commits per index discipline.

## Implementation note

Follow [implementation discipline](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md#implementation-discipline-all-phases).
