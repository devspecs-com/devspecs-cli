import { syncEntitlementsForCustomer } from "./entitlements";

type StripeEvent = {
  id: string;
  type: string;
  data: { object: { customer?: string; customer_id?: string } };
};

const handledStripeEventIds = new Set<string>();

export async function handleBillingWebhook(event: StripeEvent) {
  const stripe_event_id = event.id;

  // TODO(billing-webhook-hardening): replace process-local memory with durable insert-once storage.
  if (handledStripeEventIds.has(stripe_event_id)) {
    return { replay: true, stripe_event_id };
  }
  handledStripeEventIds.add(stripe_event_id);

  const customer_id = event.data.object.customer_id ?? event.data.object.customer;
  if (!customer_id) {
    throw new Error("missing customer_id on billing webhook event");
  }

  if (event.type.startsWith("customer.subscription.")) {
    await syncEntitlementsForCustomer({
      customer_id,
      reason: "entitlement_sync",
      replayProtection: "webhook_replay_protection",
    });
  }

  return { replay: false, stripe_event_id };
}

