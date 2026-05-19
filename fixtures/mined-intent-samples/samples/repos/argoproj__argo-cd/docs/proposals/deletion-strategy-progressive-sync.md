# Deletion Strategy for Progressive Sync

This proposal is building upon the ideas presented in https://github.com/argoproj/argo-cd/pull/14892 to introduce 
deletion strategy for progressive sync. While the original proposal laid the groundwork, this proposal extends to address
some unanswered sections and changes implementation details.

Introduce a new functionality of ArgoCD ProgressiveSync that will allow users to configure order 
of deletion for applicationSet's deployed applications. The deletion strategies can be:

- AllAtOnce (current behaviour - where all applications are deleted in no particular order without waiting for an application 
to be deleted; can be the default value)
- reverse ( delete applications in the reverse order of deployment, configured in progressiveSync. This expects the 
rollingSync field to have a specified order and implements deletion in the reverse order specified. 
Waits for one application to be fully deleted before moving onto the next application.)

## Open Questions [optional]

The original proposal mentions another strategy - `custom` wherein the user can provide a specific order of deletion. 
Is such a usecase needed? 


## Summary

This feature can extend the application dependency from deployment to deletion as well. Ability to provide deletion order 
can complete the ProgressiveSync feature.

## Motivation

Current deletion/removal strategy which ArgoCD use works fine if there aren't any dependencies between the different applications. 
However, it does not work when there are dependencies between the applications. This was noticed when some kubernetes core services 
were deployed in specific order and to be removed in reverse order.

### Goals

Following goals should be achieved in order to conclude this proposal:
1. Deletion strategy `AllAtOnce` as default value - deletes all applications at once as the current behavior of deletion.
2. Deletion strategy `Reverse` lets applications be deleted in the reverse order of the steps configured in RollingSync strategy.

### Non-Goals

custom deletion strategy - this will be a separate goal if there is enough demand for it.

## Proposal

Ability to provide configuration related to the deletion/removal process when progressive sync is used. Implementation detail provides 
two options of introducing this field in ApplicationSet. The following use cases assumes Option 1 for the yaml file examples.

### Use cases

Add a list of detailed use cases this enhancement intends to take care of.

#### AllAtOnce deletionStrategy:
<!-- stripped fenced code block: yaml -->

#### Reverse deletionStrategy:
<!-- stripped fenced code block: yaml -->
#### If custom deletionStrategy:
<!-- stripped fenced code block: yaml -->

### Implementation Details/Notes/Constraints [optional]

There should be a check that correlates the deletionStrategy to ApplicationSet strategy. For example can only select reverse 
if rollingSync lists out an order of application deployment, otherwise should error out.

It was decided to have this field within strategy (which is a field associated with progressiveSync)

To be introduced in ApplicationSetStrategy as follows:
<!-- stripped fenced code block: yaml -->

Looked at the following names for this field:
1. DeletionSyncType
2. DeletionSyncStrategy

But decided on having DeletionOrder for the following reasons:
1. simpler to understand - Order is straightforward and thus sets expectation to user
2. Since it's nested within `strategy`, suffix of strategy isn't needed.
3. Leaving room for when/if it scales to have custom deletion strategy. i.e 
<!-- stripped fenced code block: yaml -->

### Detailed examples
Already covered in Use cases

### Security Considerations

Since no additional roles or privileges are needed to be able to delete deployed applications in a specific order, 
so no impact on the security aspects of Argo CD workloads.


### Risks and Mitigations

No immediate Risks to consider


### Upgrade / Downgrade Strategy
Introducing new fields to the ApplicationSet CRD, however, no existing fields are being changed. 
This means that a new ApplicationSet version is unnecessary, and upgrading to the new spec with added fields will be a clean operation.

Downgrading could risk users receiving K8s API errors if they continue to try to apply the deletionStrategy field to a downgraded version of the ApplicationSet resource. 
Downgrading the controller while keeping the upgraded version of the CRD should cleanly downgrade/revert the behavior of the controller to the previous version without requiring users to adjust their existing ApplicationSet specs.


## Drawbacks
Slight increase in Argo CD code base complexity

## Alternatives
TBD
