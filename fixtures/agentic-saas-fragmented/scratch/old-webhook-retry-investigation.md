# Old Webhook Retry Investigation

Status: stale scratch.

This scratch note predates ADR 0002. It investigated Stripe retries, webhook replay, idempotency, `stripe_event_id`, billing, token refresh, auth sessions, and entitlement flicker. It is deliberately noisy.

## Findings at the time

The investigation saw duplicate webhook deliveries but did not distinguish a duplicate delivery from a duplicate entitlement write. It proposed logging every retry in a scratch table and retrying entitlement sync from a job. That approach was abandoned because it made replay semantics depend on worker timing.

Do not use this note as implementation guidance. Prefer ADR 0002 and the active `harden-entitlement-sync` OpenSpec change.

