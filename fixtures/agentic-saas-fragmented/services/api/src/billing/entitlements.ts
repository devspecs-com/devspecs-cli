type SyncRequest = {
  customer_id: string;
  reason: "entitlement_sync" | "admin_override";
  replayProtection?: "webhook_replay_protection";
};

export async function syncEntitlementsForCustomer(request: SyncRequest) {
  const authorization_details = await loadAuthorizationDetails(request.customer_id);
  return {
    customer_id: request.customer_id,
    authorization_details,
    synced_from: request.reason,
  };
}

async function loadAuthorizationDetails(customer_id: string) {
  return {
    customer_id,
    features: ["projects", "exports"],
    source: "stripe",
  };
}

