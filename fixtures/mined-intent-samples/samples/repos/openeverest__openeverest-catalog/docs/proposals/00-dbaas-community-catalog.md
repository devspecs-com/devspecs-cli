# DBaaS community OLM catalog

<!-- toc -->
- [DBaaS community OLM catalog](#dbaas-community-olm-catalog)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [User Stories (Optional)](#user-stories-optional)
      - [Story 1](#story-1)
    - [Notes/Constraints/Caveats](#notesconstraintscaveats)
      - [DBaaS Platform](#dbaas-platform)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [Graduation Criteria](#graduation-criteria)
    - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
    - [Version Skew Strategy](#version-skew-strategy)
  - [Production Readiness Review Questionnaire](#production-readiness-review-questionnaire)
    - [Monitoring Requirements](#monitoring-requirements)
    - [Dependencies](#dependencies)
    - [Troubleshooting](#troubleshooting)
  - [Implementation History](#implementation-history)
  - [Drawbacks](#drawbacks)
  - [Alternatives](#alternatives)
<!-- /toc -->

## Summary

<!--
This section is incredibly important for producing high-quality, user-focused
documentation such as release notes or a development roadmap. It should be
possible to collect this information before implementation begins, in order to
avoid requiring implementors to split their attention between writing release
notes and implementing the feature itself. KEP editors and SIG Docs
should help to ensure that the tone and content of the `Summary` section is
useful for a wide audience.

A good summary is probably at least a paragraph in length.

Both in this section and below, follow the guidelines of the [documentation
style guide]. In particular, wrap lines to a reasonable length, to make it
easier for reviewers to cite specific portions, and to minimize diff churn on
updates.

[documentation style guide]: https://github.com/kubernetes/community/blob/master/contributors/guide/style-guide.md
-->
OLM Catalogs are part of the Operator Lifecycle Manager ([OLM](https://olm.operatorframework.io/)) framework to provide a set of operators that extend Kubernetes. DBaaS Catalog would provide a set of operators to extend Kubernetes to deliver a DBaaS Platform based on these.

## Motivation

<!--
This section is for explicitly listing the motivation, goals, and non-goals of
this KEP.  Describe why the change is important and the benefits to users. The
motivation section can optionally provide links to [experience reports] to
demonstrate the interest in a KEP within the wider Kubernetes community.

[experience reports]: https://github.com/golang/go/wiki/ExperienceReports
-->
The Operator Lifecycle Manager ([OLM](https://olm.operatorframework.io/)) extends Kubernetes to provide a declarative way to install, manage, and upgrade Operators on a cluster. [OLM Catalogs](https://olm.operatorframework.io/docs/concepts/crds/catalogsource/) are the way to distribute a set of operators that can be installed onto a cluster via OLM.

There are some catalogs already available:
  - [OperatorHub.io](https://operatorhub.io): https://github.com/k8s-operatorhub/community-operators
  - [RH Catalogs](https://docs.openshift.com/container-platform/4.6/operators/understanding/olm-rh-catalogs.html#olm-rh-catalogs_olm-rh-catalogs): https://github.com/redhat-openshift-ecosystem/community-operators-prod

Existing catalogs include all available operators. This could be overkill for most of the systems and for developers to ensure that some sub-set of it works well together to solve particular problems.

Links:
  - https://olm.operatorframework.io/
  - https://k8s-operatorhub.github.io/community-operators/
  - https://github.com/operator-framework/cool-catalog/

### Goals

<!--
List the specific goals of the KEP. What is it trying to achieve? How will we
know that this has succeeded?
-->

Create and maintain a DBaaS catalog to provide instrumentation (catalog of operators) to build a DBaaS Platform on Kubernetes. Enable the community to maintain and extend this catalog.

### Non-Goals

<!--
What is out of scope for this KEP? Listing non-goals helps to focus discussion
and make progress.
-->

Developing operators or DBaaS Platform.

## Proposal

<!--
This is where we get down to the specifics of what the proposal actually is.
This should have enough detail that reviewers can understand exactly what
you're proposing, but should not include things like API designs or
implementation. What is the desired outcome and how do we measure success?.
The "Design Details" section below is for the real
nitty-gritty.
-->

The proposal is to create a new catalog of operators that solves one particular problem: setting up a DBaaS Platform. The scope of that catalog would be everything related to the ability to build DBaaS Platform, which includes but is not limited to the following [categories](https://github.com/k8s-operatorhub/community-operators/blob/main/categories.json):
  - Cloud Provider
  - Database
  - Logging & Tracing
  - Monitoring

Such a catalog will allow for separating different concerns, having smaller repos/images/systems, for hardening security by reducing the scope.

### User Stories (Optional)

<!--
Detail the things that people will be able to do if this KEP is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.
-->

#### Story 1

As an SRE, I would like to set up a Kubernetes cluster dedicated to database deployments and install all required operators for only that specific use case with OLM.

### Notes/Constraints/Caveats

<!--
What are the caveats to the proposal?
What are some important details that didn't come across above?
Go in to as much detail as necessary here.
This might be a good place to talk about core concepts and how they relate.
-->

#### DBaaS Platform

To set the basis for motivation, let's imagine what a DBaaS Platform could look like (so there would be an imaginary proposal for the imaginary problem).

Databases could be deployed:
  - on the same kubernetes cluster with applications
  - on the dedicated kubernetes cluster
  - on other infrastructure (VMs, HW and etc)
  - provided as the service (RDS, Aurora and etc)

Kubernetes could be a platform for the:
  - infrastructure where Databases are deployed
  - control plane that deploys Databases elsewhere (Crossplane, AWS ACK, Cluster API and etc). 

DBaaS Platform is a platform that extends Kubernetes with ability to:
  - deploy Databases and Database clusters (in any infrastructure)
  - provision infrastructure for Database \[Cluster\]
  - Observability for Databases and infrastructure
  - Management and Operations for Databases

### Risks and Mitigations

<!--
What are the risks of this proposal, and how do we mitigate? Think broadly.
For example, consider both security and how this will impact the larger
Kubernetes ecosystem.

How will security be reviewed, and by whom?

How will UX be reviewed, and by whom?

Consider including folks who also work outside the SIG or subproject.
-->

Drifts from upstreams (other catalogs, operator versions).

Version Skew between Catalogs.

## Design Details

<!--
This section should contain enough information that the specifics of your
change are understandable. This may include API specs (though not always
required) or even code snippets. If there's any ambiguity about HOW your
proposal will be implemented, this is the place to discuss them.
-->

Another proposal would be submitted for the DBaaS catalog design.

For a community-based catalog, a design should enable community and community contributions.


### Graduation Criteria

<!--
**Note:** *Not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, [feature gate] graduations, or as
something else. The KEP should keep this high-level with a focus on what
signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Feature gate][feature gate] lifecycle
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc
definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning)
or by redefining what graduation means.

In general we try to use the same stages (alpha, beta, GA), regardless of how the
functionality is accessed.

[feature gate]: https://git.k8s.io/community/contributors/devel/sig-architecture/feature-gates.md
[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

Below are some examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

#### Alpha

- Feature implemented behind a feature flag
- Initial e2e tests completed and enabled

#### Beta

- Gather feedback from developers and surveys
- Complete features A, B, C
- Additional tests are in Testgrid and linked in KEP

#### GA

- N examples of real-world usage
- N installs
- More rigorous forms of testing—e.g., downgrade tests and scalability tests
- Allowing time for feedback

**Note:** Generally we also wait at least two releases between beta and
GA/stable, because there's no opportunity for user feedback, or even bug reports,
in back-to-back releases.

**For non-optional features moving to GA, the graduation criteria must include
[conformance tests].**

[conformance tests]: https://git.k8s.io/community/contributors/devel/sig-architecture/conformance-tests.md

#### Deprecation

- Announce deprecation and support policy of the existing flag
- Two versions passed since introducing the functionality that deprecates the flag (to address version skew)
- Address feedback on usage/changed behavior, provided on GitHub issues
- Deprecate the flag
-->

None, one good feedback will be enough.

### Upgrade / Downgrade Strategy

<!--
If applicable, how will the component be upgraded and downgraded? Make sure
this is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to maintain previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to make use of the enhancement?
-->

Handled by OLM framework.

### Version Skew Strategy

<!--
If applicable, how will the component handle version skew with other
components? What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI,
  CRI or CNI may require updating that component before the kubelet.
-->

TBD. Listed in Risks. 

## Production Readiness Review Questionnaire

<!--

Production readiness reviews are intended to ensure that features merging into
Kubernetes are observable, scalable and supportable; can be safely operated in
production environments, and can be disabled or rolled back in the event they
cause increased failures in production. See more in the PRR KEP at
https://git.k8s.io/enhancements/keps/sig-architecture/1194-prod-readiness.

The production readiness review questionnaire must be completed and approved
for the KEP to move to `implementable` status and be included in the release.

In some cases, the questions below should also have answers in `kep.yaml`. This
is to enable automation to verify the presence of the review, and to reduce review
burden and latency.

The KEP must have a approver from the
[`prod-readiness-approvers`](http://git.k8s.io/enhancements/OWNERS_ALIASES)
team. Please reach out on the
[#prod-readiness](https://kubernetes.slack.com/archives/CPNHUMN74) channel if
you need any help or guidance.
-->

### Monitoring Requirements

<!--
This section must be completed when targeting beta to a release.

For GA, this section is required: approvers should be able to confirm the
previous answers based on experience in the field.
-->

TBD.

Usually operators have metrics endpoints as well as probably OLM.

Maybe additional Telemetry endpoints are needed to figure out Catalog efficiency.

### Dependencies

<!--
This section must be completed when targeting beta to a release.
-->

### Troubleshooting

<!--
This section must be completed when targeting beta to a release.

For GA, this section is required: approvers should be able to confirm the
previous answers based on experience in the field.

The Troubleshooting section currently serves the `Playbook` role. We may consider
splitting it into a dedicated `Playbook` document (potentially with some monitoring
details). For now, we leave it here.
-->

## Implementation History

<!--
Major milestones in the lifecycle of a KEP should be tracked in this section.
Major milestones might include:
- the `Summary` and `Motivation` sections being merged, signaling SIG acceptance
- the `Proposal` section being merged, signaling agreement on a proposed design
- the date implementation started
- the first Kubernetes release where an initial version of the KEP was available
- the version of Kubernetes where the KEP graduated to general availability
- when the KEP was retired or superseded
-->

## Drawbacks

<!--
Why should this KEP _not_ be implemented?
-->

A global catalog might be sufficient as it does not require that much space and maybe only needs a better API to select the desired operators based on categories.

## Alternatives

<!--
What other approaches did you consider, and why did you rule them out? These do
not need to be as detailed as the proposal, but should include enough
information to express the idea and why it was not acceptable.
-->

TBD.
