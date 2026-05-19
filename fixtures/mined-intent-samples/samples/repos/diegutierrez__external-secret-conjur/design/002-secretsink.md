<!-- stripped fenced code block: yaml -->

# Secret Sink

## Table of Contents

<!-- toc -->
// autogen please
<!-- /toc -->


## Summary
The Secret Sink is a feature to allow Secrets from Kubernetes to be saved back into some providers. Where ExternalSecret is responsible to download a Secret from a Provider into Kubernetes (as a K8s Secret), SecretSink will upload a Kubernetes Secret to a Provider.

## Motivation
Secret Sink allows some inCluster generated secrets to also be available on a given secret provider. It also allows multiple Providers having the same secret (which means a way to perform failover in case a given secret provider is on downtime or compromised for whatever the reason).

### Goals
- CRD Design for the SecretSink
- Define the need for a SinkStore
- 
### Non-Goals
Do not implement full compatibility mechanisms with each provider (we are not Terraform neither Crossplane)

### Terminology
- Sink object: any Secret (a part or the whole secret) from Kubernetes that is going to be uploaded to a Provider.
## Proposal

A controller that checks for Sink Objects, gets K8s Secrets and creates the equivalent secret on the SecretStore Provider.

### User Stories
1. As an ESO Operator I want to be able to Sync Secrets in my cluster with my External Provider
1. As an ESO Operator I want to be able to Sync Secrets even if they are not bound to a given ExternalSecret

### API
Proposed CRD changes:

<!-- stripped fenced code block: yaml -->

<!-- stripped fenced code block: yaml -->

### Behavior
When checking SecretSink for the Source Secret, check existing labels for SecretStore reference of that particular Secret. If this SecretStore reference is an object in SecretSink SecretStore lists, a SecretSyncError should be emited as we cannot sync the secret to the same SecretStore.

If the SecretStores are all fine or if the Secret has no labels (secret created by user / another tool), for Each SecretStore, get the SyncState of this store (New, SecretSynced, SecretSyncedErr).

If new Secret, or SecretSynced with refreshInterval expired, get the secret from the secretStore and see if it matches the content of the secrets. If it doesn't match, create a new secret (bumping the version, if possible) within the provider. On errors, emit SecretSyncedErr.

### Drawbacks

We had several discussions on how to implement this feature, and it turns out just by typing how many duplicate fields we would have defeated my original issue to have two separate CRDs. The biggest drawback of this solution is that it implies SecretStores to be able to write with no other mechanism available. Also, it might overload the reconciliation loop as we have 1xN secret Syncing, where most of them are actually outside the cluster.

### Acceptance Criteria
+ ExternalSecrets create appropriate labels on generated Secrets
+ SecretSinks can read labels on source Secrets
+ SecretSinks cannot have same references to SecretStores
+ SecretSinks respect refreshInterval
## Alternatives
Using some integration with Crossplane can allow to sync the secrets. Cons is this must be either manual or through some integration that would be an independent project on its own.
