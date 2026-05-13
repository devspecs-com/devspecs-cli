---
status: active
tags: [billing, support, stripe, webhook]
---

# 2026-04 Billing Ops Runbook

This runbook is a broad support artifact. It is intentionally long and noisy because real operational docs often mention every nearby concept: billing, webhook, entitlement, Stripe, customer, auth, replay, idempotency, token, session, invoices, admin overrides, and customer portal redirects.

## Purpose

Support uses this runbook when a customer says billing status looks wrong. The first step is to determine whether the issue is product visibility, Stripe state, entitlement materialization, auth/session state, or an admin override. The runbook should not be used as implementation guidance for `webhook_replay_protection`, but a broad query can easily match it.

## Triage checklist

1. Confirm the workspace `customer_id` in the billing database.
2. Check whether Stripe has an active subscription for that customer.
3. Check recent webhook deliveries and replay counts.
4. Check whether entitlement rows converged after the last subscription update.
5. Check whether `authorization_details` loaded in the auth session include the expected feature set.
6. Check whether a support admin override is active and whether it has expired.
7. Check whether the user recently visited the customer portal.
8. Check whether auth token refresh failed during billing dashboard navigation.

## Operational examples

Example A: A subscription update arrives twice. Support sees two Stripe deliveries with the same event ID. The correct behavior is for the replayed delivery to be a no-op. This runbook says how to read the dashboard, not how to implement the idempotency boundary.

Example B: A customer opens the portal and changes payment method only. No entitlement change should occur. The webhook dashboard may still show invoice events, and support should avoid escalating to engineering unless access state changes.

Example C: A workspace owner sees a stale plan badge after login. Support should compare auth/session state against entitlement rows. The session cookie may have been valid while `authorization_details` were loaded before the last entitlement materialization.

Example D: An enterprise pilot has an admin billing override. The override may grant access even when Stripe state is inactive. Support should not ask engineering to rerun `entitlement_sync` unless the Stripe-derived row is wrong.

## Repeated support guidance

The following notes repeat by design because real runbooks accumulate copy-pasted guidance.

- Billing dashboard state is not a source of truth. It is a view over Stripe-derived subscription state, entitlement rows, admin overrides, and auth/session materialization.
- Stripe event replay is normal. A replayed webhook should not scare support by itself.
- Idempotency for webhook processing belongs at the handler boundary, while idempotency for customer portal session creation belongs at the portal endpoint boundary.
- The `customer_id` is the safest join key for support investigations, but it is not an auth token and must not be copied into browser-visible credentials.
- `authorization_details` are derived data. If they are stale, investigate the materializer and session reload path before changing billing logic.
- Local entitlement caching was explored and superseded. Do not direct support to edit local cache rows.
- Token refresh belongs to auth infrastructure. Billing webhook handlers do not issue tokens or invalidate cookies.

## Escalation guide

Escalate to billing engineering when Stripe state and entitlement rows disagree for more than five minutes. Escalate to auth engineering when session validation succeeds but `authorization_details` are missing or inconsistent. Escalate to product when portal UX is confusing but system state is correct. Escalate to support tooling when dashboards cannot explain webhook replay or admin override state.

## Known false positives for search

This runbook contains many words that look relevant to implementation:

- `stripe_event_id`
- `webhook_replay_protection`
- `entitlement_sync`
- `authorization_details`
- `customer_id`
- auth token
- session cookie
- customer portal
- idempotency
- replay

Those mentions are diagnostic, not normative. An eval case that asks for implementation context should usually exclude this runbook even though it shares many terms.

## Long notes from support drills

During an April support drill, the team replayed several customer scenarios. One scenario involved a subscription update replay, a customer portal return, a session refresh, and a stale dashboard badge. The resolution was to read the webhook event table, confirm that the duplicate event had already been acknowledged, reload the workspace session, and then compare the derived authorization record.

Another scenario involved a support override expiring during a portal session. The customer saw access drop after returning from Stripe, but the underlying billing state was unchanged. The fix was to explain override expiry and update internal support copy. No webhook, token, or entitlement sync code changed.

A third scenario involved duplicate Stripe customers after a migration. The customer portal opened for the old customer record, while entitlements were derived from the new record. The migration notes, not the webhook hardening design, were the correct context.

These operational details are intentionally realistic distractors. They are useful for support triage but too broad for targeted agent implementation context.

