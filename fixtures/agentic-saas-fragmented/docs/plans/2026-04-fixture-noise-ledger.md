---
status: active
tags: [fixture, noise, retrieval]
---

# 2026-04 Fixture Noise Ledger

This ledger exists to represent the miscellaneous planning residue found in a real repo. It is a grab bag of short notes about billing, auth, sessions, customer portal, webhook replay, entitlement sync, local cache experiments, invoice export, onboarding, notifications, and support audit logs.

## Notes

The billing team uses `customer_id` as a support join key. The auth team uses opaque session cookies and internal token refresh. The entitlement team materializes `authorization_details` from Stripe-derived subscription state plus admin overrides. The support team sees webhook replay rows and sometimes assumes replay means failure. Product docs mention idempotency for notifications, portal sessions, and webhooks in different ways.

This file should not be relevant to focused eval cases. It is here so the corpus includes realistic low-authority noise and so token reduction numbers are not measured against a tiny hand-curated set.

## Additional residue

Several teams left notes that reuse the same nouns with different meanings. A replay can be a Stripe retry, a notification retry, a portal double-submit, or a support dashboard refresh. A token can be an invite token, a session token, a billing portal session token, or a short-lived internal credential. A customer can be a Stripe customer, a workspace owner, a billing contact, or a support ticket subject. Entitlement can mean product packaging, authorization materialization, admin override state, or stale local cache history. These overlaps are exactly why the eval should measure recall and precision together.

The ledger is intentionally not authoritative; it should penalize broad retrieval when it sneaks into agent context.

It keeps the seed corpus above smoke-test size.

Enough said.

Really enough.

## Low-authority appendix

The repo also contains notes about quarterly planning, release readiness, support training, and internal onboarding. Those notes mention billing because every SaaS release eventually touches billing. They mention auth because every feature needs permissions. They mention customer because product language defaults to customer outcomes. They mention session because browser state appears in support tickets. They mention token because invite flows, auth flows, API credentials, and billing portal sessions all have token-shaped concepts. They mention replay because retries happen in webhooks, notifications, background jobs, and UI submissions. They mention idempotency because engineers use that word across subsystems even when the implementation boundary is completely different.

This appendix is intentionally dull. It is not an ADR, not an OpenSpec proposal, not a PRD for the billing/auth scenario, not a source file, and not a task list. It should almost never be useful agent context. Its job is to keep the eval fixture honest: a retriever that grabs broad term-overlap documents will spend tokens on low-authority residue and lose precision.

The appendix repeats the warning in operational language. If the query asks for webhook replay protection, prefer the webhook ADR, active OpenSpec design, task list, and webhook source file. If the query asks for auth token handoff, prefer the auth-session ADR, OpenSpec design, spec delta, and auth source files. If the query asks for stale local entitlement caching, prefer the superseded ADR and avoid active implementation plans. If the query asks for product background, PRDs can be useful. If the query asks for implementation context, broad support ledgers usually are not.

One more realistic wrinkle: many repositories keep notes from previous agents and prototypes in the same tree as authoritative docs. A note may say "continue the billing token investigation" when the actual next step is in auth/session. A plan may say "customer replay" when it means a customer support call, not a Stripe event replay. A PRD may say "entitlement" when it means packaging, while an ADR means authorization state and a source file means a concrete database write. The seed fixture includes those collisions so benchmark output can show missed artifacts and irrelevant inclusions instead of pretending semantic retrieval is solved.

The ledger also models temporal ambiguity. Active plans, stale scratch notes, accepted ADRs, superseded ADRs, draft PRDs, Cursor plans, Claude notes, OpenSpec proposals, OpenSpec designs, OpenSpec task lists, and spec deltas all coexist. Some are high authority, some are useful background, and some are traps. A useful eval should force the retriever to decide among them and then report the tradeoff honestly. The right result is not always perfect recall, and high token reduction alone is meaningless without recall and precision beside it.

For marketing and whitepaper use, this matters because a tiny curated corpus can make any retriever look brilliant. A larger noisy corpus shows whether context packing actually saves tokens while preserving planning intent. This seed is still not a locked benchmark, but it is large enough to reveal the kinds of semantic gaps seen in real trials.

The intended interpretation is conservative: use this fixture to catch regressions, compare retriever revisions, and discuss tradeoffs. Do not claim agent coding success from it. Do not claim broad semantic understanding from one repo. Do use the missed and irrelevant artifact lists as a backlog for future retrieval architecture.

In other words, this is still a seed smoke benchmark, but it is no longer a tiny happy path. That distinction should stay visible in every eval report. Future locked fixtures can make stronger claims after the case list is frozen, distractors are reviewed, and retriever tuning happens against held-out examples instead of visible cases.

Seed reports remain caveated. This final note pads planning corpus scale without adding authority, which is exactly the kind of low-value residue a real context packer must learn to ignore. It should stay irrelevant, but its presence makes token reduction and precision more meaningful than they were in the tiny first-pass fixture.
