---
status: draft
kind: requirements
subtype: prd
tags: [notifications, workspace]
---

# PRD: Workspace Notifications v1

Workspace Notifications v1 adds in-app notifications for owners and admins. It is mostly orthogonal to billing/auth, but it includes common SaaS vocabulary that can confuse simple retrieval systems.

## Goals

- Notify owners when invite acceptance completes.
- Notify admins when billing portal settings change.
- Notify support-visible admins when invoice export fails.
- Notify owners when trial limits are near.
- Notify admins when workspace permissions change.

## Non-goals

- Do not process Stripe webhooks.
- Do not implement entitlement sync.
- Do not refresh auth tokens.
- Do not update session cookies.
- Do not write `authorization_details`.

## Notification sources

Notifications may be triggered by onboarding events, invite events, billing dashboard events, customer portal returns, support audit events, and workspace role changes. They may read high-level billing state, but they should never read raw Stripe payloads or raw auth token values.

## User stories

As a workspace owner, I see a notification when a teammate accepts an invite. As a billing admin, I see a notification when invoice export fails. As a support user, I can see whether a notification came from portal, onboarding, billing, auth/session, or admin audit state.

## Implementation notes

Notification delivery should be idempotent per notification key, but that idempotency is not `stripe_event_id` idempotency. It is a local delivery guarantee. This is another example where the word idempotency alone is not enough to select the right planning artifact.

## Retrieval warning

Queries containing billing, token, session, customer, portal, idempotency, replay, entitlement, or authorization may match this PRD. It should only be relevant when the task asks about notifications or product background for workspace messaging.

## Acceptance criteria

- Notification keys are stable.
- Duplicate delivery is suppressed.
- Notification payloads do not include secrets.
- Billing and auth events are referenced by opaque IDs.
- Owners can dismiss notifications.
- Support can inspect delivery history.

