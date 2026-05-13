# Old Billing Plan

Status: stale scratch, do not implement.

This old billing sketch proposed a local entitlement caching plan where `authorization_details` would be updated from a cron job instead of Stripe webhooks. It mentions `customer_id`, `stripe_event_id`, and `entitlement_sync`, but it predates ADR 0001 and ADR 0002.

The plan is superseded and should usually be excluded from agent context unless the task is specifically to audit stale scratch notes.

