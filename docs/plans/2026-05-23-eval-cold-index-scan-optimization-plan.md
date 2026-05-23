# Eval Cold Index Scan Optimization Plan

Date: 2026-05-23

## Goal

Reduce cold indexed eval time for real repositories when test-case and code-comment intent artifacts are enabled.

## Problem

The current indexed eval path runs multiple repo walks and file reads through independent adapters, then persists every discovered artifact one by one. Manual checks against real50 timeout/slow repos show filesystem eval completes in under a second while indexed test/comment corpus construction takes tens of seconds to timeout.

## Implementation

1. Add an optional file-based adapter interface for adapters that can discover candidates from a shared file inventory.
2. Make scanner builds optionally share one deterministic repo file inventory across file-based adapters.
3. Read each matching file once per shared discovery pass and fan it out to source-context, test-case, and code-comment extraction.
4. Parallelize uncapped file extraction, then sort results deterministically before parsing/upserting.
5. Let eval pass deterministic pre-parse candidate caps to the scanner for source, test-case, and code-comment artifacts.
6. Wrap eval-only scan writes in a transaction and skip per-artifact git authored-date lookups for temporary eval databases.

## Success Criteria

- Existing `ds scan` behavior remains compatible by default.
- `ds eval --json` with indexed corpus still emits the same artifact shapes plus the existing additive telemetry/budget fields.
- Eval budgets cap source/test/comment candidates before parse/upsert work.
- Uncapped file-based discovery uses parallel workers but produces stable artifact ordering.
- A cold indexed eval smoke on a previously slow repo is materially faster than the previous test/comment path.
- Targeted Go tests pass for eval harness, scanner, and command flag plumbing.

## Rollback Criteria

- Cached and uncached eval results diverge for the same cache key.
- Scanner output ordering becomes nondeterministic.
- Production `ds scan` drops artifacts relative to the adapter-local discovery path.
- Eval budgets silently affect runs when no budget flags are provided.
