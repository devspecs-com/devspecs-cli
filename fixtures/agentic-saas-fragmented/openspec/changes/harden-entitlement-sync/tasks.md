# Tasks: Harden Entitlement Sync

- [x] Add migration for `billing_webhook_events` keyed by `stripe_event_id`.
- [ ] Replace in-memory webhook duplicate detection with durable insert-once storage.
- [ ] Update `services/api/src/billing/webhooks.ts` to branch replayed events before `entitlement_sync`.
- [ ] Ensure entitlement sync persists `authorization_details` for the session materializer.
- [ ] Add tests for duplicate `stripe_event_id` and out-of-order Stripe retries.
- [ ] Document operational behavior for `webhook_replay_protection`.

