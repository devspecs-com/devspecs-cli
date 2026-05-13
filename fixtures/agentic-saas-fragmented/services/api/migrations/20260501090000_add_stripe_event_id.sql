-- Durable webhook replay protection for billing-webhook-hardening.
CREATE TABLE billing_webhook_events (
  stripe_event_id TEXT PRIMARY KEY,
  customer_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX billing_webhook_events_customer_id_idx
  ON billing_webhook_events (customer_id);

