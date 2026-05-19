---
title: Mined Sample Promotion Plan
kind: plan
status: draft
tags: [eval, classifier, indexing, mined-samples, rfc, openspec]
---

# Mined Sample Promotion Plan

## Purpose

Use the mined sample corpus to improve DevSpecs first-index eval quality without turning noisy GitHub mining results into hidden training data.

The immediate goal is to promote a small, verified slice from the recent sample-miner target-10 run into deterministic DevSpecs regression fixtures. These fixtures should raise the measured precision, recall, and context sufficiency of the first index path while preserving token reduction versus handing an AI agent the whole repository context.

The marketing artifact is the eval. The product story should be:

```text
Install DevSpecs locally, run the first index, and give AI agents materially better context with fewer tokens.
```

The mined samples are useful only insofar as they make that eval more representative, reproducible, and persuasive.

Source corpus for the first pass:

```text
devspecs-sample-miner/intent_corpus_prod_20260518-101648
```

## North Star

This work is eval-led. The north star is not a broad "wow moment" narrative; it is a measurable first-index result that can be used in marketing and product proof:

- high classifier precision on real co-located engineering intent artifacts
- strong recall for common high-value artifacts, especially RFCs, ADRs, PRDs, OpenSpec bundles, BMAD/story artifacts, and agent plans
- high context sufficiency for AI-agent tasks
- large token reduction versus full repository or full planning-corpus context
- deterministic local reproduction from committed fixtures and saved eval outputs

A good public claim should be based on a command a user can run locally, not on anecdotal examples. The eval should show that DevSpecs reduces literal agent token cost while preserving or improving useful engineering context.

Kleio-style provenance, contradiction, and staleness analysis can build on this later. For this plan, those are secondary to first-index quality metrics.

## Measurement Priorities

The promoted fixture should improve and protect these measurements:

1. **Precision:** indexed artifacts should actually be engineering intent, not random markdown.
2. **Must-have recall:** common high-value artifacts should be present in the index after the first scan.
3. **Context sufficiency:** retrieved context should answer eval tasks without needing the full repo.
4. **Token reduction:** retrieved context should be materially smaller than full planning context.
5. **Fixture path coverage:** eval cases must resolve to real committed fixture paths or explicit metadata-only records.
6. **Regression visibility:** every miss or false positive should be explainable from saved eval output.

Do not chase recall by indexing noisy markdown indiscriminately. For marketing, a smaller high-precision first index with clear token savings is more valuable than a broad index that bloats context.

## Ground Rules

- Treat mined samples as candidate evidence, not ground truth.
- Manually verify every promoted sample label.
- Do not write classifier rules for a specific repository, file hash, or memorized fixture.
- Every classifier or parser change must cite the reusable feature pattern it relies on.
- Keep provenance for every promoted sample: source URL, repository, commit SHA, original path, license, promoted label, and review notes.
- Commit full real files only when license-compatible.
- Use reduced synthetic derivatives when the original content is useful but not redistributable.
- Keep non-redistributable full samples outside the repo and commit metadata only.
- Keep API specs separate from OpenSpec-the-framework. API specs may remain useful, but they must not drive OpenSpec classifier behavior.

## Sample Scope Model

The fixture model needs to avoid treating every markdown file as the same kind of sample.

### Document Sample

A single markdown or text-like artifact that should stand alone for classification and parsing.

Examples:

- ADR markdown file
- PRD markdown file
- RFC markdown file
- Cursor plan file
- Claude plan file
- Codex plan file
- generic implementation plan
- BMAD-like story markdown file
- API spec markdown file

### Bundle Sample

A bounded group of files that together represent one workflow artifact.

Initial OpenSpec target should be counted as bundles, not files. One OpenSpec change bundle commonly contains:

```text
openspec/changes/<change-id>/proposal.md
openspec/changes/<change-id>/design.md
openspec/changes/<change-id>/tasks.md
openspec/changes/<change-id>/specs/<capability>/spec.md
```

Archived OpenSpec changes should preserve the extra archive segment:

```text
openspec/changes/archive/<change-id>/...
```

### Collection Sample

A larger root structure containing multiple bundles, base specs, conventions, or archive state.

For OpenSpec, this is typically an `openspec/` directory that may include:

```text
openspec/specs/**/spec.md
openspec/changes/*/...
openspec/changes/archive/*/...
openspec/project.md
```

Collections should be preserved when available because they test repository-level indexing behavior. They should not replace the initial bundle target.

## Initial Promotion Targets

These are small regression targets, not claims about corpus prevalence.

| Family | Initial Target | Scope | Notes |
| --- | ---: | --- | --- |
| RFC | 10 | document | Treat as first-class early because RFCs appear to be common co-located design artifacts. |
| ADR | 5 | document | Include at least one strict Nygard/MADR/Y-statement-style sample if available. |
| PRD | 5 | document | Prefer product intent docs with scope, users, requirements, or acceptance criteria. |
| OpenSpec | 5 | bundle | Count change bundles, not individual files. Include archive layout if available. |
| OpenSpec collection | 1-2 | collection | Optional first pass; useful when a mined repo preserves multiple related changes/specs. |
| BMAD | 5 | document or bundle | Strict BMAD only when evidence supports official/structured BMAD shape. |
| BMAD-like story | 5 | document | Story-shaped artifact without claiming official BMAD. |
| Cursor plan | up to 5 | document | Use synthetic reduced derivatives if redistributable full files are scarce. |
| Claude plan | up to 5 | document | Include `.claude` path conventions and continuation/task-plan shapes. |
| Codex plan | up to 5 | document | Include Codex-authored plans and staged work notes if available. |
| Generic plan | up to 5 | document | Broad implementation/migration/rollout plan shapes. |
| API spec | 5 | document | Keep as separate API_SPEC family and negative contrast for OpenSpec. |

If a family has fewer than the target number of license-compatible full files, fill the fixture with reduced synthetic derivatives plus provenance metadata rather than weakening the label standard.

## Fixture Layout

Create a new mined fixture that can run through the existing eval harness without polluting the synthetic SaaS fixture:

```text
fixtures/mined-intent-samples/
  README.md
  manifest.yaml
  classifier_cases.yaml
  repos/
    adr-prd-rfc/
    openspec-framework/
    agent-plans/
    bmad-stories/
    api-specs/
    false-positives/
```

Use runnable mini-repos under `repos/` when discovery or adapter behavior matters. Use individual document samples only when classification/parsing can be tested without repository layout.

Keep non-redistributable metadata in a separate manifest section; do not copy full file contents into the fixture.

## Eval Diagnostics

Keep these as low-lift diagnostic overlays on the existing `ds eval` path. Do not create a parallel eval framework unless the current harness becomes a blocker.

The primary marketing metrics remain:

- token reduction
- artifact precision
- artifact recall
- must-have recall
- context sufficiency

The diagnostics below should explain why those numbers move.

### Discovery Diagnostic

Verifies that scan/index finds the artifact or container at all. In the current eval, this is mostly visible through missed expected artifacts and corpus/index contents.

Examples:

- `openspec/changes/<id>` is discovered as a container/bundle.
- `openspec/changes/archive/<id>` is discovered without collapsing all archives into one bundle.
- `.cursor/plans/**`, `.claude/**`, `.codex/**`, and `*.plan.md` candidates are discovered.
- RFCs under `rfcs/`, `docs/rfcs/`, and `docs/proposals/` are discovered.

### Classification Diagnostic

Verifies the expected classifier, subtype, family, format profile, authority, and negative boundaries.

Required early boundaries:

- OpenSpec framework vs API_SPEC
- BMAD strict vs BMAD-like story
- ADR vs accidental `adr` substring paths
- RFC vs generic proposal/readme noise
- agent-authored plan vs generic markdown fallback

### Structure Diagnostic

Verifies related files remain linked through layout groups, child candidates, or source metadata.

Initial OpenSpec checks:

- proposal/design/tasks/spec deltas share one layout group for an active change bundle
- archived changes keep their specific change ID
- collection fixtures preserve root/base specs separately from change bundles

### Parser Diagnostic

Verifies normalized extraction quality from promoted samples.

Early fields:

- title
- status/lifecycle
- todo/checklist items
- acceptance criteria
- headings/section roles
- source paths and layout groups

## Implementation Phases

### Phase 0: Baseline The Current Eval

- Run and save the current first-index eval before adding mined fixtures.
- Record precision, recall, must-have recall, context sufficiency, token reduction, and worst-case misses.
- Treat this baseline as the comparison point for every classifier/indexing change.

### Phase 1: Promotion Manifest and Fixture Skeleton

- Add `fixtures/mined-intent-samples/README.md`.
- Add `fixtures/mined-intent-samples/manifest.yaml` with provenance and review status.
- Add `fixtures/mined-intent-samples/classifier_cases.yaml` with initial promoted cases.
- Make the fixture runnable by the existing eval path or a clearly named mined-sample eval command.
- Keep the current staged plan documents untouched.

### Phase 2: Promote Verified Samples

- Review the target-10 mined corpus by family.
- Promote RFCs early, alongside ADR/PRD, because RFCs appear common and valuable for co-located engineering intent.
- Promote 5 OpenSpec change bundles, not 5 OpenSpec markdown files.
- Preserve OpenSpec collections when a source repo provides useful multi-bundle/base-spec context.
- Add negative/distractor cases from false positives where they teach a reusable boundary.

### Phase 3: Wire Eval Coverage

- Keep `ds eval` as the primary measurement path.
- Add classifier eval coverage only where it explains or protects first-index precision/recall.
- Add discovery/adapter tests where a full mini-repo is required, especially for OpenSpec companion files and RFC locations.
- Add parser assertions only for fields that materially affect retrieval or context sufficiency.
- Ensure eval output reports before/after movement in precision, recall, must-have recall, context sufficiency, and token reduction.
- Add low-lift diagnostic labels or summaries only if they make the existing eval easier to interpret.
- Use saved eval JSON files as the marketing-facing evidence trail.

## External Agent Audit Criteria

An external coding agent should be able to verify the first implementation pass from repository state and deterministic command output. The audit should not require private context from this thread.

| Criterion | Evidence To Inspect | Audit Command |
| --- | --- | --- |
| Existing eval remains the primary marketing artifact | `ds eval` output includes token reduction, precision, recall, must-have recall, and sufficiency | `go test ./internal/evalharness ./internal/commands` and `go run ./cmd/ds eval ./fixtures/agentic-saas-fragmented --json --no-save` |
| Diagnostics do not create a second eval framework | New fields are part of the existing eval JSON/text result, not a separate command path | `go run ./cmd/ds eval ./fixtures/agentic-saas-fragmented --json --no-save` |
| Discovery gaps are auditable | Each case reports expected artifacts missing from the indexed/eval corpus separately from artifacts that were indexed but not retrieved | Inspect JSON fields for `expected_missing_from_corpus`, `missed_after_discovery`, `discovery_coverage`, and `retrieval_coverage_of_discovered` |
| Role/family gaps are auditable | Eval output summarizes expected/retrieved/missing counts by inferred artifact role such as `openspec_design`, `openspec_tasks`, `rfc`, `adr`, `prd`, and `agent_note` | Inspect JSON `diagnostics.role_summaries` and text `Diagnostics` sections |
| Token-savings claim remains visible | Summary still reports token reduction beside precision/recall/sufficiency | Inspect text `Summary` and JSON `summary` |
| Existing fixture still runs deterministically | Existing tests pass without requiring network or private sample corpora | `go test ./internal/evalharness ./internal/commands ./internal/classify` |
| User-staged plan files are not overwritten | Only this plan plus implementation files should be changed by this pass | `git status --short` |

## Initial Implementation Success Criteria

- `go test ./internal/evalharness ./internal/commands ./internal/classify` passes locally.
- `go run ./cmd/ds eval ./fixtures/agentic-saas-fragmented --json --no-save` emits `summary` and `diagnostics` objects.
- At least one current eval case exposes indexed-coverage gaps through `expected_missing_from_corpus` or `missed_after_discovery`.
- Text eval output includes a short `Diagnostics` section with discovery coverage and role/family gap summaries.
- The implementation does not add a new eval command, new eval fixture requirement, or network dependency.

## Acceptance Criteria For This First Pass

- The mined fixture can be run deterministically in local tests and eval commands.
- The plan preserves the existing `ds eval` path as the primary marketing artifact.
- First-index precision, recall, context sufficiency, and token reduction are explicit tracked outcomes.
- Additional diagnostics explain those outcomes without creating a separate eval framework.
- RFCs are represented as a first-class early family.
- OpenSpec target is expressed as 5 bundles, with optional collections tracked separately.
- Every promoted sample has provenance and review status.
- Non-redistributable samples are metadata-only or reduced synthetic derivatives.
- Classifier expectations are written in terms of feature patterns, not source-specific memorization.
- Existing staged `docs/plans/*.md` files are not modified by this work.

