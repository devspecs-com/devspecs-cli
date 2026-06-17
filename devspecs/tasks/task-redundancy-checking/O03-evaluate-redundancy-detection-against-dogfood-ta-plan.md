# Task task-redundancy-checking O03 Plan

## Goal
Evaluate redundancy detection against the dogfood task corpus.

## Claim
This feature should not ship until we know whether overlap warnings are helpful signal or annoying topic-level noise.

## Evaluation Corpus
Use local dogfood task workspaces first:

- DevSpecs CLI `devspecs/tasks/**`
- at least one internal dogfood repo with a known redundant-plan incident
- optional synthetic fixtures only for edge cases that are hard to find naturally

## Labels To Collect
- true duplicate: same intended implementation already open
- related but valid: same subsystem, different planned change
- historical context: closed or superseded work, useful as context only
- route conflict: work is valid but in the wrong order
- false positive: broad theme overlap without actionable redundancy
- false negative: human notices overlap the detector missed

## Metrics
- warning precision on open/unimplemented slices
- false-positive rate for broad theme overlap
- number of warnings per task/list run
- percent of warnings with concrete evidence paths
- percent of warnings that name a usable next command

## Acceptance Checks
- [ ] Dogfood examples include at least one real duplicate or near-duplicate.
- [ ] False positives are reviewed before promotion.
- [ ] The eval distinguishes useful related work from redundant work.
- [ ] Output remains short enough for CLI use.
- [ ] Results decide whether to implement, improve, rework, or defer.

## Decision Gates
- Promote if warnings catch real open-work duplication with acceptable precision.
- Improve if the feature is useful but needs better state filtering or evidence receipts.
- Rework if warnings mostly rediscover broad themes.
- Rollback if warnings would make agents ignore valid parallel plans.
- Block if no real dogfood examples can be found.
