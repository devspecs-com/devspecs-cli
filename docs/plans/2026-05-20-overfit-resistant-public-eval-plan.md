# Overfit-Resistant Public Eval Plan

Date: 2026-05-20

## Goal

Turn the current promising first-index eval into a public, third-party-auditable eval that supports the north-star claim:

> DevSpecs can be installed locally, index an existing repo on first pass, and give AI agents useful engineering intent context at materially lower input-token cost.

The core concern this plan addresses is overfitting: synthetic fixture bias, miner/discovery bias, path-label leakage, and rules tuned to known examples.

## Current Starting Point

Current internal eval signals are useful but not yet public-grade:

- Retrieval/token fixture: `fixtures/agentic-saas-fragmented`
- Mean token reduction: about 94.0%
- Mean retrieval precision: about 85.8%
- Mean artifact recall: about 83.9%
- Mean must-have recall: about 93.2%
- Context sufficiency: 10/11
- Discovery coverage: 100.0%
- Mined classifier holdout: `fixtures/mined-intent-samples`
- Mined classifier accuracy: about 96.3%

Interpretation: classifier quality is already credible, but agent utility is still under-proven because retrieval is tested on too few cases and too much of the corpus is shaped by our own fixture design.

## Public Eval Principles

1. Separate tuning data from public holdout data by repository, not by file.
2. Freeze a lockbox holdout before using it for product changes.
3. Preserve provenance for every real sample.
4. Include hard negatives and near misses, not only obvious ADR/PRD/RFC/OpenSpec files.
5. Evaluate retrieval utility separately from classification accuracy.
6. Report sample sizes, confidence intervals where practical, and known weaknesses.
7. Compare against simple baselines so token savings is not measured in a vacuum.
8. Treat mined samples as eval data, not hidden training data for fixture-specific rules.

## Data Splits

Create three repo-disjoint pools:

- `dev`: examples we inspect freely and use while improving classifiers/retrieval.
- `validation`: examples we can run often while tuning, but use sparingly for manual inspection.
- `public_holdout`: frozen examples used for final reporting and not inspected during implementation except through aggregate eval output.

Split by repository slug so files from one repo cannot appear in multiple pools.

Recommended initial target sizes:

- Classifier dev: 30-50 labeled artifacts per major class when available.
- Classifier validation: 20-30 labeled artifacts per major class.
- Public classifier holdout: at least 20 labeled artifacts per major class, plus negatives.
- Retrieval validation: 10-15 manually labeled tasks across 3-5 repos.
- Public retrieval holdout: 15-25 manually labeled tasks across 5-10 repos.

OpenSpec should be counted by bundle/collection for retrieval and by individual child document for classifier coverage.

## Provenance Requirements

Each real sample or bundle must carry machine-readable metadata:

- source URL
- repository slug
- commit SHA
- original path
- license
- redistributability decision
- artifact label
- artifact scope: file, bundle, collection, negative
- discovery query or source mechanism
- reduction/synthetic rewrite notes, if any
- manual reviewer and review timestamp, when labeled

Redistributability paths:

- license-compatible full files can live under real sample fixtures.
- reduced synthetic derivatives can live under synthetic fixture directories.
- non-redistributable full files stay outside the repo; only metadata and labels are committed.

## Step 1: Inventory Existing Evidence

Audit current committed eval assets and recent miner output:

- List all current classifier fixtures and labels.
- List all current retrieval cases and expected artifacts.
- Count examples by artifact type, repository, license class, and discovery source.
- Identify samples that came from the same repository as tuning examples.
- Identify labels that rely too heavily on filenames or paths.

Deliverable:

- A small inventory report committed under `docs/eval/` or emitted by a script.

Success criteria:

- We can state how many examples exist per class and per split.
- We can identify which current numbers come from synthetic vs mined data.

## Step 2: Add Split Metadata And Lockbox Mechanics

Add explicit split metadata to classifier and retrieval fixtures.

Expected shape:

```yaml
fixture_version: mined-intent-samples-v1
eval_stage: real_mined_holdout_v1
split: validation
label_policy: manual_reviewed
```

For public holdout fixtures:

```yaml
split: public_holdout
locked_at: 2026-05-20T00:00:00Z
locked_by: manual
change_policy: append_only_with_review_note
```

Add a guardrail to eval tooling:

- Warn when running public holdout with dirty fixture files.
- Record commit SHA of the evaluated fixture repo.
- Include split name in JSON and text eval reports.

Deliverable:

- Fixture metadata support in eval JSON/text.
- Public holdout fixtures clearly marked.

Success criteria:

- External agents can tell which numbers are tuning numbers and which are holdout numbers.

## Step 3: Build Negative And Near-Miss Sets

Add negative cases to reduce miner/classifier bias:

- README files with project architecture sections.
- Changelogs and release notes.
- API/OpenAPI specs that are not OpenSpec framework artifacts.
- Docs with `adr` as a substring but no ADR semantics.
- Plans that are generic notes, not agent plans.
- Tutorials, runbooks, install docs, and troubleshooting docs.
- RFC-looking files that are protocol docs rather than engineering intent RFCs.

For classifier eval, negatives should assert `generic_markdown` or another explicit non-target label.

Deliverable:

- Negative classifier fixture cases with provenance.

Success criteria:

- Public classifier metrics include false-positive pressure, not only recall over obvious positives.

## Step 4: Create Manual Labeling Protocol

Create a short labeler guide that defines each class by observable features, not by repository identity.

For each label, document:

- positive criteria
- required or strongly indicative sections
- allowed variants
- common false positives
- examples of borderline cases
- how to label bundles/collections versus child files

For each manually labeled sample, record:

- reviewer
- decision
- confidence: high, medium, low
- notes for borderline cases

Deliverable:

- `docs/eval/labeling-protocol.md`
- Metadata fields for reviewer/confidence/notes.

Success criteria:

- A third-party agent can label a new sample consistently from the protocol.

## Step 5: Construct Repo-Disjoint Classifier Holdout

Use mined samples plus manual review to build a frozen classifier holdout.

Artifact classes to prioritize:

- ADR
- RFC
- PRD
- OpenSpec framework bundle and child docs
- agent plans: Cursor, Claude, Codex, generic plans
- BMAD and BMAD-like story artifacts, if kept in scope
- API spec as a separate class or negative/near-miss lane, not conflated with OpenSpec framework

Rules:

- No repository may overlap with dev or validation sets.
- No query-specific hidden rules may be added after lock without annotating the eval result as invalidated.
- If labels are corrected after lock, record correction notes and publish both before/after counts when relevant.

Deliverable:

- `fixtures/public-classifier-holdout/` or equivalent.

Success criteria:

- At least 20 examples per high-priority class where enough data exists.
- At least 50 hard negatives/near misses.

## Step 6: Construct Real Retrieval Holdout

Classifier accuracy does not prove agent utility. Add retrieval tasks over real or semi-real repos.

Each retrieval case should include:

- user/agent query
- expected relevant artifacts with importance: must, helpful, background
- expected excluded artifacts
- sufficiency criteria
- baseline corpus token counts
- manual rationale for why the expected artifacts matter

Case themes:

- trace a bug or feature to ADR/RFC/PRD context
- find stale or conflicting plan context
- retrieve OpenSpec proposal/design/tasks/spec bundle context
- distinguish API spec from OpenSpec framework context
- retrieve agent plan state without flooding with every markdown file

Deliverable:

- Retrieval holdout fixture with 15-25 real-world cases.

Success criteria:

- Public eval can answer: did DevSpecs retrieve enough context for an agent to proceed?

## Step 7: Add Baselines

Compare DevSpecs against simple alternatives:

- all planning markdown
- all markdown
- filename/path grep
- query-term retrieval
- optionally BM25 or lightweight lexical retrieval if low lift

Metrics per baseline:

- input tokens
- precision
- recall
- must-have recall
- sufficiency pass rate

Deliverable:

- Existing first-index report includes baseline comparison table or JSON fields.

Success criteria:

- Public result shows DevSpecs is not merely smaller, but smaller while preserving useful context better than naive methods.

## Step 8: Report Statistical Shape

Add confidence-aware reporting where practical.

Minimum reporting:

- sample counts per lane and class
- precision/recall by class
- macro and micro averages
- false-positive rate on negatives
- weak cases and confusion matrix

Nice-to-have:

- Wilson intervals for precision/recall/accuracy
- bootstrap interval for mean token reduction

Deliverable:

- First-index JSON report has enough structure for a dashboard or external audit.

Success criteria:

- Marketing claims can be phrased with honest denominators.

## Step 9: CI And Release Discipline

Add CI jobs or local scripts for:

- dev eval: can run on every PR
- validation eval: can run on every PR or nightly
- public holdout eval: run before release or before publishing updated claims

Avoid using public holdout failures as a direct tuning loop. If a public holdout result causes a change, log the reason and consider rotating in a fresh holdout batch.

Deliverable:

- `scripts/run-public-eval` or equivalent.
- CI documentation for when each lane runs.

Success criteria:

- Public eval numbers are reproducible from a clean checkout.

## Step 10: Publishable Eval Artifact

Create one command that produces the public artifact:

```sh
ds eval ./fixtures/public-retrieval-holdout \
  --first-index-report \
  --classifier-fixture ./fixtures/public-classifier-holdout \
  --input-usd-per-1m <model-price> \
  --json
```

The text report should be readable by humans. The JSON report should be suitable for a static page, blog post, or marketing chart.

Deliverable:

- Public eval report generated from frozen fixtures.

Success criteria:

- A third-party reviewer can reproduce the reported numbers and inspect the methodology.

## Immediate Next Implementation Slice

Do this first:

1. Add fixture split metadata support to classifier and retrieval eval outputs.
2. Add an inventory script/report for existing fixtures and mined samples.
3. Add a label protocol doc.
4. Promote a small negative/near-miss classifier set.
5. Create a validation retrieval fixture from mined/recent examples before freezing public holdout.

This order improves trust without overcommitting to a large public dataset before the mechanics are auditable.

## Decision Gates

Before publishing public claims, require:

- Public classifier holdout has repo-disjoint labels and negatives.
- Public retrieval holdout has at least 15 manually reviewed cases.
- First-index report includes baselines and saved-token counts.
- No class has tiny hidden denominators in the headline claim.
- Known weak cases are documented rather than buried.

## Open Questions

- Should BMAD-like story artifacts be reported as official BMAD, BMAD-like, or a broader story artifact class?
- Should API spec stay as a separate classifier label or only as an OpenSpec negative/near-miss lane?
- What public input-token price should be used for cost-savings examples?
- How often should public holdout rotate as mined corpus grows?
