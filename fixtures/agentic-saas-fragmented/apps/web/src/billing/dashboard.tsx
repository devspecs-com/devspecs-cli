type EntitlementSummary = {
  customer_id: string;
  plan: "free" | "team" | "enterprise";
  seats: number;
  status: "active" | "past_due" | "trialing";
};

export function BillingDashboard({ summary }: { summary: EntitlementSummary }) {
  return (
    <section>
      <h1>Billing</h1>
      <dl>
        <dt>Customer</dt>
        <dd>{summary.customer_id}</dd>
        <dt>Plan</dt>
        <dd>{summary.plan}</dd>
        <dt>Seats</dt>
        <dd>{summary.seats}</dd>
      </dl>
    </section>
  );
}

