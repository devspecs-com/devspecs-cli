---
status: active
tags: [pnpm, desktop, migration]
---

# 260219 pnpm Migration

This dated plan mirrors the real-world issue where filename slugs and date prefixes carry most of the retrieval signal. The plan has little to do with billing, but it mentions auth tokens and customer sessions because package migration touched desktop login tests.

## Plan

- Move the desktop workspace from npm to pnpm.
- Update lockfile handling and cache directories.
- Confirm auth/session integration tests still pass after dependency resolution changes.
- Verify billing dashboard stories still compile in the desktop shell.
- Avoid changing Stripe, entitlement sync, customer portal, or webhook replay code.

## Retrieval purpose

Queries for `pnpm` or "dated plan" should find this file even though the title starts with a compact date slug and the path lives under `apps/desktop/docs/plans`, a location not covered by the older default markdown paths.

