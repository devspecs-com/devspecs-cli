type PortalRequest = {
  customer_id: string;
  returnUrl: string;
};

export async function createCustomerPortalSession(request: PortalRequest) {
  return {
    customer_id: request.customer_id,
    url: `https://billing.stripe.test/session/${request.customer_id}`,
    returnUrl: request.returnUrl,
  };
}
