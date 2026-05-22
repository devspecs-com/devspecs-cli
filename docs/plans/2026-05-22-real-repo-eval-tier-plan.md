# Real Repo Eval Tier Plan

Date: 2026-05-22

## Goal

Make real-repository retrieval performance measurable in the same shape as the canonical first-index eval, without treating the small real dev set as a public benchmark too early.

## Current Data

The miner has enough raw material for real-repo evaluation:

- `repo-sampling-splits-500.json`: 816 unique repositories, with dev / validation / lockbox splits.
- Candidate paths: 10,929 total across the split manifest.
- Existing real retrieval labels: 5 repositories / 15 first-pass cases.
- Existing manual OpenSpec labels: 80 file-level OpenSpec labels.

This is enough to start a real-repo eval tier, but not enough for stable public claims. The current real retrieval labels are a development seed. The next target should be 50 reviewed retrieval cases, then 100+ before using the numbers externally.

## Approach

Keep three eval layers distinct:

1. Canonical fixture: synthetic/curated, committed, used for CI regression and headline smoke.
2. Real dev set: ignored local repo fixtures with first-pass labels; used for tuning and diagnosis.
3. Real validation/lockbox sets: repo-disjoint ignored fixtures; used for promotion checks and public-confidence dry runs after labels are reviewed.

The immediate implementation is a batch first-index report mode for `ds eval`:

- Accept one root containing repo fixture directories with `cases.yaml`.
- Discover fixtures either directly under the root or under `repos/`.
- Run the existing indexed eval harness for each fixture.
- Aggregate north-star metrics across all cases.
- Emit JSON/text with per-fixture diagnostics and weak spots.
- Save per-fixture eval results under the existing `--results-dir` unless `--no-save` is set.

## Acceptance Criteria

External agents can audit success without conversation context:

- `ds eval <real-set-root> --first-index-report --batch-fixtures --json --no-save` returns one aggregate report.
- The aggregate report includes total cases, weighted precision/recall/must-have recall, context sufficiency, discovery coverage, token totals, saved tokens, and per-fixture retrieval summaries.
- The command fails clearly when no child fixtures contain `cases.yaml`.
- Existing single-fixture `ds eval --first-index-report` behavior remains unchanged.
- Existing eval tests pass.

## Follow-Up Fixes

After the real-repo batch report is available, apply only narrow classifier/retrieval patches that improve the fixed dev slice and then validate on repo-disjoint validation:

- RFC/enhancement README recall: recognize mature proposal families in `enhancements/**/README.md` and EP-style documents without naming specific repos.
- ADR vs protocol conflict: ADR path plus ADR title/status/decision sections should beat generic protocol standard language.
- Retrieval ranking: optimize only after discovery/classification misses are visible in batch reports.

## Guardrails

- Do not optimize against lockbox.
- Do not add repo-specific rules, exact title rules, or hash/path exceptions.
- Do not hide discovery failures by ranking unrelated markdown higher.
- If a patch does not improve fixed dev metrics or hurts validation materially, revert it.
