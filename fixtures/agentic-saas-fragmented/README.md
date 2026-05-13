# Agentic SaaS Fragmented Fixture

This fixture is a deliberately partial TypeScript/React + API SaaS repository for DevSpecs evals.

Domain: billing/auth SaaS.

Main scenario: harden billing webhook idempotency and entitlement sync without reopening the old local entitlement cache design.

Important identifiers appear across planning and source files:

- `stripe_event_id`
- `entitlement_sync`
- `webhook_replay_protection`
- `customer_id`
- `authorization_details`
- `billing-webhook-hardening`
- `harden-entitlement-sync`

## Eval Caveat

This seed eval is deterministic and filesystem-based. It measures context retrieval/compression, not agent answer quality. The seed fixture validates the harness; benchmark claims should use locked fixtures with distractors and no case-specific tuning.
