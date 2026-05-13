---
status: active
tags: [support, search, taxonomy]
---

# 2026-04 Support Search Taxonomy

This taxonomy defines the words support uses when searching customer incidents. It is deliberately broad and overlaps with many implementation terms.

## Terms

Billing means invoices, subscriptions, customer portal sessions, payment methods, and plan state. Auth means session cookies, token refresh, invite tokens, workspace membership, and authorization checks. Entitlement means derived access state, feature flags, plan limits, and admin overrides. Webhook means Stripe delivery, replay, event type, processing status, and idempotency. Customer can mean workspace owner, paying account, Stripe customer, support contact, or product user.

## Guidance

Support search should ask clarifying questions before escalating. A ticket that says "token failed after billing change" may be an auth/session issue, a customer portal redirect issue, a stale authorization details issue, or a billing entitlement convergence issue. A ticket that says "webhook replay" may only mean a dashboard row showed duplicate delivery. A ticket that says "customer id mismatch" may require Stripe migration notes, not entitlement sync implementation.

## Retrieval purpose

This file exists to make naive keyword retrieval look overly confident. It contains `stripe_event_id`, `customer_id`, `authorization_details`, `entitlement_sync`, webhook replay, idempotency, billing, auth, token, session, portal, admin override, invoice export, onboarding, notification, support dashboard, and local entitlement caching language. It is almost never the right artifact for targeted agent context.

## Taxonomy examples

- "billing token" should not automatically mean auth token.
- "portal replay" should not automatically mean webhook replay protection.
- "customer session" may refer to product usage or Stripe customer portal.
- "authorization details stale" may refer to auth/session materialization, entitlement sync, or local cache history.
- "idempotent delivery" may refer to notifications, webhooks, or portal session creation.

The taxonomy is useful for humans triaging support language. It should usually be excluded from implementation context packs.

