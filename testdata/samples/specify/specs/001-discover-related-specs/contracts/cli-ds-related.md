# CLI contracts: `ds workon`, `ds mine`, `ds related`

**Module**: `github.com/devspecs-com/devspecs-cli`  
**Binary**: `ds`  
**Feature**: [spec.md](../spec.md)

This document captures **user-facing / script-facing** contracts. Exact Go struct tags must match here in tests (golden or snapshot) when `--json` is used.

---

## `ds workon`

### Usage

- `ds workon <id>` — start session (resolves full ID, short ID, or prefix per existing artifact resolution).
- `ds workon` — print active session for current repo/worktree/branch or message if none.
- `ds workon --clear` — end active session.

### Human-readable output (non-JSON)

- Success start: includes **branch** and resolved **artifact id** (per codex PLAN example wording).
- Status: shows **artifact id** + metadata or “none”.

### Flags

| Flag | Semantics |
|------|-----------|
| `--clear` | Close active session for current scope |

*(Add `--json` only if implementation chooses parity with other commands; spec does not require JSON for workon — if added, document fields in tests.)*

---

## `ds mine`

### Usage

- `ds mine` — default: current repo, **recent**-style scope when no flag (align with hook expectations; exact default must match help text).
- `ds mine --recent` — limit to merge-base / nearby commits per [research.md](../research.md) R-3.
- `ds mine --all` — broader history with **hard caps** (document maxima in flag help or `ds mine --help`).
- `ds mine --quiet` — suppress non-essential stdout on success (required for hooks).
- `ds mine --json` — machine-readable **summary**, including bucket / evidence counts as required by FR-007.

### JSON (`--json`) — indicative fields

**Stability**: field names MUST remain backward compatible within minor CLI releases unless release notes declare a breaking change.

| Field | Type | Meaning |
|-------|------|---------|
| `repo_root` | string | Absolute repo root mined |
| `scope` | string | `"recent"` \| `"all"` (or enum used in code) |
| `commits_scanned` | int | Number of commits processed (after caps) |
| `links_written` | int | Upserts performed |
| `bucket_counts` | object | Counts keyed by `high`, `medium`, `low`, or `"non_match"` for below threshold |
| `warnings` | array of string | Optional truncation / cap notices |

Implementations MAY add additive fields (`duration_ms`, `head_commit`) if tests lock them.

---

## `ds related`

### Usage

- `ds related <path>` — file argument; repo-relative or resolvable absolute under repo root.
- `ds related <path> --all` — include **low** bucket matches.
- `ds related <path> --json` — machine-readable results.

### Human-readable output

For each **artifact** in default view (high + medium only):

- Artifact identity (id + title if available from existing `show` conventions).
- **Bucket** label.
- **Evidence lines**: one line per contributing signal (type + human `evidence_value`).

### JSON (`--json`) — indicative top-level shape

```json
{
  "file_path": "normalized/repo/relative/path",
  "results": [
    {
      "artifact_id": "string",
      "score": 0.0,
      "bucket": "high|medium|low",
      "evidence": [
        {
          "evidence_type": "string",
          "evidence_value": "string",
          "confidence": 0.0,
          "first_observed_at": "RFC3339",
          "last_observed_at": "RFC3339"
        }
      ]
    }
  ]
}
```

**Rules**:

- `results` sorted by **`score`** descending.
- Omit below-threshold aggregates entirely unless `--all`, consistent with FR-005.
- **`score`** is post-cap aggregate in `[0,1]`.

---

## Git hooks (`ds init --hooks`)

Installed hooks MUST match [research.md](../research.md) R-5. Re-invoking `ds init --hooks` MUST NOT duplicate hook blocks (marker idempotency).

---

## Compatibility

Structured output is consumed by automation; add fields additively. Renaming or removing fields requires explicit CLI semver / deprecation policy coordinated with downstream scripts.
