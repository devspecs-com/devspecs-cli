---
status: stale
tags: [stripe, billing, cleanup]
---

# 2026-04 Legacy Stripe Cleanup

This plan is a distractor for billing and Stripe searches. It predates `billing-webhook-hardening` and mostly covers old customer metadata cleanup after a migration from multiple Stripe products to a single product family.

## Context

The old billing integration wrote `customer_id` into three places: account metadata, workspace billing settings, and a support-only lookup table. Several records also stored an obsolete `stripe_customer` field. The cleanup was meant to normalize those references before the customer portal work began.

The plan mentions webhook delivery, replay, idempotency, auth, and entitlement in passing because every billing document does. It does not define the idempotency boundary for `stripe_event_id`, does not describe `entitlement_sync`, and should not be used as implementation context for webhook replay protection.

## Notes

- Backfill missing `customer_id` values from Stripe metadata exports.
- Compare portal-created customers against historical checkout-created customers.
- Remove the abandoned `stripe_customer` alias only after support dashboards stop reading it.
- Keep billing retry logs for 90 days so support can audit replay behavior.
- Do not add new entitlement behavior in this cleanup; access state comes from the newer entitlement sync flow.
- Do not change `authorization_details`; this cleanup is about customer identity hygiene.

## Long-form migration checklist

1. Export Stripe customers created before April 2026 and group them by workspace domain.
2. Verify each export row has a canonical `customer_id` and a billing contact email.
3. Reconcile workspaces where multiple customer records share the same domain but not the same subscription.
4. Annotate cases where portal sessions created duplicate customers during testing.
5. Run a dry-run script that prints proposed customer merges without writing to production.
6. Confirm that webhook logs for subscription updates still point at the surviving customer record.
7. Ask support to sample ten enterprise accounts and ten self-serve accounts.
8. Remove dashboard filters that depend on obsolete metadata.
9. Keep an audit artifact in the billing folder so later agents do not confuse this with `harden-entitlement-sync`.

## Why this is not the active webhook work

This cleanup is about legacy Stripe customer identity. It shares terms with the active webhook hardening work, but it does not define durable insert-once processing for `stripe_event_id`, does not change `webhook_replay_protection`, and does not describe the current entitlement source of truth.

