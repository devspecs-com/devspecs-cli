---
name: Phase 2 — scan output (human + JSON)
overview: ds scan breakdown by source_type + format_profile; human labels Planning docs / OpenSpec / ADRs; JSON adds sources_breakdown while keeping found.
todos:
  - id: p2-labels-map
    content: Central human labels map (markdown openspec adr) for output layer only.
    status: completed
  - id: p2-scan-result
    content: Extend scan.Result + aggregate format counts inside Scanner.Run (no extra DB query).
    status: completed
  - id: p2-human-output
    content: "Human ds scan: Indexed by source + format breakdown; --quiet unchanged."
    status: completed
  - id: p2-json-output
    content: "ds scan --json: keep found; add sources_breakdown; document fields."
    status: completed
  - id: p2-tests
    content: "Tests: golden/CLI text + JSON unmarshal + sums; optional determinism."
    status: completed
  - id: p2-readme
    content: "README: short labels vs source_type note; optional taxonomy subsection."
    status: completed
  - id: p2-commits
    content: Incremental commits per index discipline.
    status: completed
isProject: false
---

# Phase 2: Scan output

**Goal**: Make `ds scan` **trustworthy and legible** without changing indexing semantics. Depends on phase 1 aggregation fields being available on each ingested item.

## Tasks

- [x] **Labels map**: central place for human labels (`markdown` → “Planning docs”, `openspec` → “OpenSpec”, `adr` → “ADRs”) used only by output layer — [`internal/scan/labels.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/labels.go).
- [x] **`scan.Result`**: extend with per–`source_type` counts + `map[format_profile]count` (or ordered slice); build inside [`Scanner.Run`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go) while iterating upserts (no extra DB pass) — [`internal/scan/result.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/result.go).
- [x] **Human `ds scan`**: replace flat “Found” block with **Indexed by source** + format breakdown; keep `--quiet` silent — [`internal/commands/scan.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/scan.go).
- [x] **JSON `ds scan --json`**: keep **`Found`** (same wire key as before; see note below); add **`sources_breakdown`** with `source_type`, `label`, `count`, `formats`; documented on `Result` in code + README “Scan summaries”.
- [x] **Tests**: CLI / unit coverage — `scan_output_test.go`, extended `acceptance_test.go` DOD scan, `scan` package tests for aggregation and multi-format.
- [x] **README**: short “Scan summaries” subsection under Core workflow — [`README.md`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/README.md).
- [x] **Commits**: incremental (scan package → commands → README).

## Human output

Replace flat “Found: N markdown” with **two-level** summary, e.g.:

```text
Indexed by source:
  Planning docs   3   formats: …   ← formats line is alphabetical by profile (stable), not necessarily this order
  OpenSpec         1
  ADRs             2
```

Illustrative layout — the **`formats:`** segment uses **alphabetical** profile keys (e.g. `cursor_plan`, `generic`, `speckit`), not the order shown in older prose examples.

Rules:

- **Stable internal** id remains `markdown` / `openspec` / `adr`; **human row labels**: “Planning docs”, “OpenSpec”, “ADRs” (exact capitalization TBD).
- Aggregate during [`Scanner.Run`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go) from in-memory counts — no extra DB query.
- `--quiet` suppresses the **human** summary (see **`--json`** behavior above — JSON mode never prints the human block).

## JSON output (`ds scan --json`)

**Backward compatibility**: keep the existing flat per-adapter map. The JSON field name is **`Found`** (capital `F`, from `encoding/json` on `scan.Result`) — same as pre–phase-2 clients already used.

**Add** parallel structure:

```json
{
  "Found": { "markdown": 3, "openspec": 1, "adr": 2 },
  "sources_breakdown": [
    {
      "source_type": "markdown",
      "label": "Planning docs",
      "count": 3,
      "formats": { "cursor_plan": 1, "speckit": 1, "generic": 1 }
    },
    {
      "source_type": "openspec",
      "label": "OpenSpec",
      "count": 1,
      "formats": { "openspec": 1 }
    },
    {
      "source_type": "adr",
      "label": "ADRs",
      "count": 2,
      "formats": { "adr": 2 }
    }
  ]
}
```

- **`ds scan --json`** must **never prompt** (unchanged contract).

## Optional follow-ons (same phase if small)

- `ds show` / `ds list --json`: surface `format_profile` (and later `layout_group`).
- `ds scan --verbose`: one line that ignores are applied (if phase 3 landed) — can wait.

## README

- **Simple** user-facing wording in quick start; **deep** taxonomy in a subsection (“How DevSpecs classifies artifacts”) — avoid hero wall-of-ontology per GPT feedback.

## Auditable success criteria (phase 2)

- [x] Non-JSON `ds scan` output labels the markdown pipeline row as **Planning docs** (or final chosen label), not the bare word `markdown`; **OpenSpec** and **ADRs** rows use agreed labels.
- [x] Output still shows **per-format counts** under Planning docs when multiple `format_profile` values exist — `TestScan_SourcesBreakdown_MultipleMarkdownFormats` in [`internal/scan/scan_test.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan_test.go).
- [x] `ds scan --json`: unmarshaling succeeds; top-level **`Found`** with keys `markdown`, `openspec`, `adr` and integer counts matching total artifacts indexed per adapter.
- [x] **`sources_breakdown`** present: three rows (fixed order); each element has `source_type`, `label`, `count`, `formats`; sum of `count` across rows equals total artifacts indexed.
- [x] **`sources_breakdown` formats** counts sum to `count` per row — asserted in `TestDOD_03_ScanArtifacts` / `TestResult_finalizeSourcesBreakdown_Sums`.
- [x] `ds scan --quiet` emits **no** human summary lines — `TestScanQuiet` in [`internal/commands/freshness_test.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/freshness_test.go). **`--json`** never emits the human block; **`TestScan_QuietWithJSON_WritesJSONSuppressesHuman`** documents parity for `--json --quiet`.
- [x] No stdin prompts; two consecutive JSON scans after a warmup scan produce **identical** full JSON — `TestScanJSON_ConsecutiveRunsIdentical` in [`internal/commands/scan_output_test.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/scan_output_test.go).
- [x] Incremental commits per index discipline.

## Implementation / audit notes (contract vs. code)

These are **not** plan failures; they document behavior for external auditors and future multi-source pipelines.

1. **`tallyIndexed` / `sources[0]`** — Breakdown `source_type` and `format_profile` are taken from the **first** `Source` in the slice. Today every adapter returns a **single** primary source; if a future adapter returned multiple sources with different `format_profile` values, only the first would drive `sources_breakdown` (tallies would not automatically split per source row).

2. **`finalizeSourcesBreakdown` row order** — The three breakdown rows are **fixed**: `markdown`, `openspec`, `adr`. A **new** pipeline name would still increment **`Found`** for that adapter, but would **not** get its own `sources_breakdown` row until this list (and labels) are extended in code.

3. **Human `formats:` line** — Profile keys are rendered in **alphabetical** order (`sort.Strings` in `formatScanFormatsHuman`) for **stable** output. The plan’s prose example order (e.g. `cursor_plan` before `generic`) is illustrative only.

4. **`--quiet` + `--json`** — When **`--json`** is set, `runScan` writes JSON and **returns immediately** (no human summary). **`--quiet`** therefore only affects the human path: `ds scan --quiet` stays silent; `ds scan --json --quiet` is equivalent to `ds scan --json` for stdout (both omit human text). The **`--quiet`** flag remains meaningful for **stderr** side channels (e.g. `verbose && !quiet` during `--rebuild`). The Cobra **`--quiet`** help text states it is redundant when **`--json`** is set.

## Implementation note

Follow [implementation discipline](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md#implementation-discipline-all-phases).
