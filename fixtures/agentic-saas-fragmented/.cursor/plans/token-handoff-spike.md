---
status: stale
generator: cursor-plan
tags: [auth, token, spike]
---

# Cursor Plan: Token Handoff Spike

This stale spike proposed passing a short-lived auth token through web routes, including billing and customer portal routes. It is superseded by ADR 0005 and the `refactor-auth-session` OpenSpec change.

## Spike idea

- Browser requests a short-lived token during login.
- Billing dashboard stores the token in route state.
- Customer portal redirect returns with token state preserved.
- Entitlement-derived `authorization_details` are refreshed client-side after billing changes.

## Problems found

The design mixed auth/session behavior with billing UX. It made token refresh visible to frontend code, encouraged copying `customer_id` into claims, and made webhook replay appear related to session invalidation. It also created noisy traces where a Stripe event, portal redirect, and token refresh happened in the same support window.

## Current status

Do not implement this spike. Read ADR 0005 and `refactor-auth-session` instead. This file exists as a semantic distractor for queries that mention auth token, session, billing dashboard, portal, entitlement, and authorization details.

