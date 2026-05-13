---
status: active
source: claude-notes
tags: [webhook_replay_protection]
---

# Claude Notes: Webhook Idempotency Follow-up

The previous agent stopped after adding the migration. Resume by replacing the Set-based replay check in `services/api/src/billing/webhooks.ts`.

Important detail: `webhook_replay_protection` should return 2xx for an existing `stripe_event_id`, but should not call `entitlement_sync`. This keeps Stripe retries quiet while avoiding duplicate entitlement writes for the same `customer_id`.

