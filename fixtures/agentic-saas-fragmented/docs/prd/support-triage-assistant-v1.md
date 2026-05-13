---
status: draft
kind: requirements
subtype: prd
tags: [support, triage]
---

# PRD: Support Triage Assistant v1

The support triage assistant helps support staff classify tickets before escalating to engineering. It is product planning, not engineering implementation context.

## Goals

- Classify tickets as billing, auth/session, customer portal, entitlement, notification, onboarding, audit log, or invoice export.
- Ask whether a ticket mentions `customer_id`, `stripe_event_id`, `authorization_details`, token refresh, session cookie, portal redirect, admin override, local entitlement cache, or webhook replay.
- Link support staff to the right runbook without claiming that keyword overlap proves causality.

## Non-goals

- Do not implement semantic retrieval for DevSpecs.
- Do not process Stripe webhooks.
- Do not update entitlement rows.
- Do not change auth token refresh.
- Do not write customer portal code.

## Why this is a distractor

The assistant PRD intentionally contains the same vocabulary as many implementation tasks. It mentions billing, webhook, entitlement, Stripe, customer, auth, replay, idempotency, token, session, customer portal, invoice export, onboarding, and audit logs. A keyword retriever may select it often, but it should only be relevant when the user asks for support triage product background.

## Acceptance Criteria

- Support can classify a ticket without reading raw Stripe payloads.
- Support can distinguish auth token issues from billing entitlement issues.
- Support can see when a portal redirect and webhook replay happened near the same time.
- Support is warned that local entitlement caching plans are stale.
- Support sees links to authoritative ADRs and OpenSpec changes when escalation is needed.

