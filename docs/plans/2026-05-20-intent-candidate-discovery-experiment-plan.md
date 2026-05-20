# Intent Candidate Discovery Experiment Plan

Date: 2026-05-20

## Goal

Improve first-index recall for engineering intent artifacts without overfitting to mined repository examples.

The current classifier pipeline can score document evidence once a file enters the scan pipeline, but candidate discovery is still dominated by exact configured/default paths. Real split-set repos show useful planning and design artifacts under varied path conventions, so the experiment should broaden candidate discovery using general path/content signals and then measure whether indexed results improve.

## Hypothesis

An opt-in markdown candidate discovery pass based on normalized path tokens and shallow markdown structure will increase useful intent-artifact discovery while keeping precision acceptable.

Examples from mined repos should motivate feature classes only, not exact rules. A path such as `docs/exec-plans/active/foo.md` should be selected because it contains a normalized `plan` token and likely plan headings, not because `exec-plans` is hard-coded.

## Experiment Boundaries

- Keep default `ds scan` behavior unchanged.
- Gate the new discovery mode behind an explicit CLI flag and repo config experiment switch.
- Preserve existing configured path discovery.
- Respect `.gitignore`, `.git/info/exclude`, and `.aiignore`.
- Use hard directory skips only for operational trees that are already unsafe/noisy to scan deeply, such as `.git`, dependency directories, and build outputs.
- Prefer positive scoring and downweighting over broad semantic directory exclusion.
- Persist discovery reasons in classifier metadata so eval reports can explain why a file entered the pipeline.

## Candidate Signals

Path/token positives:

- Strong: normalized tokens related to plan, design, proposal, decision, adr, rfc, requirement, spec, architecture.
- Medium: implementation, migration, rollout, task, story, epic, milestone, roadmap, risk.
- Agentic: agent, claude, cursor, codex, gemini, copilot.

Content positives from shallow markdown reads:

- Headings or title snippets such as Goals, Non-goals, Context, Decision, Alternatives, Implementation Plan, Tasks, Acceptance Criteria, Risks, Rollout, Open Questions.

Negative/downweight signals:

- Root README, changelog, license, code of conduct, security policy, contributing docs, pull request templates, release notes, and generated markers.

## Implementation Steps

1. Add an experiment switch:
   - `ds scan --experimental-intent-discovery`
   - `.devspecs/config.yaml` support under `experiments.intent_candidate_discovery`.
2. Extend markdown discovery with an opt-in broad walk over markdown files.
3. Add a generic scorer that tokenizes path segments robustly across separators, camelCase, and simple plurals/stems.
4. Read a bounded prefix of candidate files for heading/title/content evidence.
5. Add `DiscoveryReason` and `DiscoveryScore` metadata to candidates and persist them in classifier metadata.
6. Add unit tests proving generic patterns are discovered and common noisy files are not selected by the experiment.
7. Run baseline and experiment scans over a small split-set smoke, then compare indexed artifact counts and paths.

## Measurement

For each experiment run, capture:

- baseline scan JSON
- experimental scan JSON
- indexed artifact paths
- classifier metadata reasons for newly indexed artifacts
- split-set repo identifiers used

Keep the experiment only if it clearly improves candidate recall on real split-set repos without flooding root-level maintenance docs.

## Rollback Rule

This change should be easy to revert as a single experiment if validation does not improve. Do not mix classifier rule tuning or retrieval ranking changes into the same commit.
