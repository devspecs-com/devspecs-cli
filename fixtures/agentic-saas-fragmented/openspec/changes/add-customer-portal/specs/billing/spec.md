## ADDED Requirements

### Requirement: Customer portal session

Workspace owners MUST be able to create a Stripe-hosted customer portal session for the current `customer_id`.

#### Scenario: owner opens portal

- **GIVEN** a valid auth session
- **WHEN** the owner opens billing portal
- **THEN** the API creates a portal session
- **AND** does not mutate entitlement rows directly

