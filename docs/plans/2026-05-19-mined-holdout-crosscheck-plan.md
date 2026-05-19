# Mined Holdout Cross-Check Plan

Date: 2026-05-19

## Goal

Turn the current real mined sample corpus into an auditable holdout fixture for `devspecs-cli` classifier work. The fixture should let us cross-check the synthetic/local eval against real GitHub artifacts without quietly training fixture-specific rules into the classifier.

The near-term target is 30-50 real samples per high-value category where the miner already found license-compatible files. We should promote what is available now, keep provenance attached, and make the promotion step repeatable so later miner runs can refresh the fixture.

## Current Sample Availability

From `devspecs-sample-miner/intent_corpus_prod_20260518-192521/testdata/classifier-samples/real`:

- ADR: 24 redistributable full files.
- RFC: 42 redistributable full files.
- PRD: 13 redistributable full files.
- BMAD: 17 redistributable full files.
- BMAD-like story: 4 redistributable full files.
- API spec: 4 redistributable full files.
- OpenSpec: many redistributable files, but individual files are less useful than preserving change bundles.

## Promotion Rules

1. Commit only samples whose miner metadata says the full file can be committed.
2. Keep source URL, repository, commit SHA, license, original path, format label, and reduction notes in the fixture manifest and classifier cases.
3. Prefer repository-diverse selection over lexicographic selection, so a single prolific repository does not dominate early numbers.
4. Make OpenSpec a bundle/container case, not a flood of unrelated individual markdown files.
5. Include only classifier labels the current deterministic classifier actually owns in `classifier_cases.yaml`.
6. Store unsupported or not-yet-modeled labels as provenance-bearing samples in the fixture manifest, not classifier assertions.

## First Fixture Shape

Create `fixtures/mined-intent-samples/`:

- `README.md`: fixture purpose, source run, refresh command, and policy.
- `cases.yaml`: fixture labels for the eval runner.
- `classifier_cases.yaml`: real-sample classifier assertions.
- `manifest.yaml`: all promoted samples and bundle metadata, including unsupported labels.
- `samples/repos/<owner>__<repo>/<original path>`: promoted full-file samples and OpenSpec bundle trees, preserving original path cues instead of leaking the expected type through fixture folders.

## Classifier Case Scope

Assert now:

- ADR documents as `adr`.
- RFC/enhancement/proposal documents as `rfc`.
- PRD documents as `prd`.
- OpenSpec change bundles as `openspec` container cases with child candidates for proposal, design, tasks, and spec deltas.

Do not assert yet:

- BMAD as a classifier label. The CLI currently has a BMAD format profile in adapter/format code, but no first-class classifier model named `bmad`.
- API spec as a classifier label. The CLI should keep API specs separate from OpenSpec framework support before making this an eval assertion.
- BMAD-like story as a classifier label. It needs a deliberate taxonomy decision: official BMAD output, BMAD-compatible story artifact, or generic implementation story.

## Success Criteria

External agents should be able to audit this without conversation context:

- The promotion script can regenerate the fixture from a miner output directory.
- `classifier_cases.yaml` includes provenance for every asserted real sample.
- `ds eval fixtures/mined-intent-samples --classifier --no-save` runs successfully.
- The eval output identifies real-sample classifier misses separately from the synthetic `agentic-saas-fragmented` fixture.
- The manifest records unsupported promoted labels so they are visible but not treated as hidden classifier training labels.

## Guardrails Against Overfitting

- Do not add classifier rules that cite a specific repository, hash, title, or original sample path.
- Do not store classifier cases under type-named fixture paths such as `samples/adr/`; preserve the original repository-relative path so path evidence stays realistic.
- When a real holdout case fails, describe the missing general feature pattern first.
- Promote additional samples before tightening a model if the failure pattern is based on only one or two examples.
- Keep OpenSpec bundle validation container-oriented, because file-level OpenSpec documents can be proposal, design, tasks, or spec delta roles inside one higher-level change.

## Implementation Steps

1. Add a repeatable promotion script under `scripts/`.
2. Generate `fixtures/mined-intent-samples/` from the current miner run.
3. Run the mined classifier eval and record the initial real-sample accuracy.
4. Run the existing synthetic classifier and retrieval evals to ensure the holdout fixture does not regress current evals.
5. Commit the plan, script, fixture, and any small classifier fixes that are justified by broad feature patterns.
