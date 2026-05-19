# Enable pre-caching of user-specified images

## Summary

The existing pre-caching feature on the Topology Aware Lifecycle Manager (TALM) allow users to
precache OpenShift specific workloads on managed clusters. This enhancement proposal (EP) suggests
adding a new feature to TALM to allow users to pre-cache additional application-specific workload
images on managed clusters.

This EP also serves as an opportunity to refactor the pre-caching functionality in TALM and future-proof
the design.

## Motivation

The bandwidth between typical SNO sites and the centralized registry(es) is limited:
- Less than 100 Mbps in some cases
- Round trip packet latency may also be high

Furthermore, aggregate application workload image sizes to be transferred during an upgrade can be high.

Customers would like to pre-cache additional workload images on the SNO sites prior to upgrading their
application(s). This reduces the impact of slow networks and removes risk due to transient networking
events during the upgrade.


### User Stories

- As a user, I want to pre-cache my application images so that I can reliably and efficiently upgrade
my clusters.

### Goals

The user must be able to do the following:
- Declaratively define a set of images to be pre-cached on managed SNO nodes.
- Enable pre-caching of that set of images to one or more nodes at a time of their choosing (e.g.
maintenance window).
- Retrieve status indicating success or failure of the pre-cache operation.

The proposed solution should not increase steady-state CPU use of cluster outside of the precaching
operation.


### Non-Goals

The pre-caching and backup functionality of TALM is currently enabled to work on SNOs only. There is
an item on the enhancement list to extend this functionality to operate on multi-node architectures
(SNO+workers). This EP will also target SNOs only and the capability for multi-node configurations
will be scoped as a future enhancement.

The following items are out of scope for this EP:
- As there is no requirement for an image registry on the spoke cluster, there will be no user interface
(UI) for inspecting the cached images. This implies that there will be no support for modifying/deleting
existing pre-cached images on the spoke clusters.
- Management of the user application is not included. This epic comprises only pre-caching
user-specified images in CRIO.
- There will be no additional status / conditions added to the current pre-caching feature. The pre-caching job
does not communicate back to TALM - it only returns a success or failure. The failure can occur at any stage of the
pre-caching job and is therefore non-trivial for TALM to derive the cause of failure. Should there be a requirement
for enhanced fault-handling, then a separate EP should be created. However, it is important to note that any error
incurred during the pre-cachig stage will be adequately captured in the cluster logs.


## Proposal

### Overview

The proposed solution is to leverage the existing pre-caching job on TALM to pre-cache the additional
user-specified images. This job is currently used to pre-cache the OpenShift platform image and operator
images on the managed clusters. The job runs on the clusters and pulls the requested set of missing
images/layers.

This EP further allows the application of the existing Cluster Group Upgrade (CGU) custom resource (CR)
to enable the user to pre-cache the additional images.

The proposed approach consists of introducing a new custom resource definition (CRD) `PreCachingConfig`
for specifying the configuration options for the pre-caching job. This new pre-caching config CR will be
referenced in the Cluster Group Upgrade (CGU) CR. The`PreCachingConfig`CR replaces the current solution
of using a ConfigMap (named `cluster-group-upgrade-overrides`) to specify the pre-caching configurations.
The ConfigMap will be deprecated starting with this release.

An example of the proposed pre-caching configuration CR is shown below.

<!-- stripped fenced code block: yaml -->

It is important to note that all the fields in the CR are optional. Also worthy to note that the
`platformImage`, `operatorsIndexes` and `operatorsPackagesAndChannels` fields have been collectively
grouped under the `overrides` object. This is done to indicate to the user that these values need not
be specified as they are automatically derived from the policies pertaining to the managed clusters.
Furthermore, it also provides the context of the user overriding the default TALM functionality. Should
there be new fields in the future which are automatically computed by TALM, these can be added here as
well. The CR also contains 2 new fields, namely `spaceRequired` and `additionalImages`.

The `additionalImages` field allows the user to specify the list of additional images they desire to be
pre-cached. This field specifically addresses the topic of the EP. The `spaceRequired` field allows the
user to specify the minimum required disk space (specified in Gigabytes) on the cluster. TALM defines a
default value for OpenShift related images if the parameter is unspecified. Prior to pulling the images,
the pre-caching job performs a check to verify whether there is enough disk space to avoid triggering
kubelet to perform garbage collection (GC) and thus mitigate the benefit of the pre-caching job.
The `spaceRequired` value should take into account the disk space required by both OpenShift related images
as well as the additional user images.

TALM reports the pre-caching status in the CGU as shown below. Although the `platformImage`, `operatorsIndexes`
and `operatorsPackagesAndChannels` fields are nested under the `PreCachingConfig.spec.overrides` object, it
is important to note that they will still be shown as a flat json object under `status.precaching` in the CGU CR.

<!-- stripped fenced code block: json -->

The pre-caching CR needs to be referenced in the CGU. There are three potential ways in which this can be
done. Each option carries its own merrit and challenges.

**Option 1:**
This option is the simplest solution. It entails creating a new top-level field under the CGU `spec`
object to reference the pre-caching config CR. In this approach, the `PreCachingConfig` CR is referenced
in the CGU under the field name `preCachingConfigRef`. The `preCaching` field is still used to enable the
pre-caching job.

This approach is shown below.

<!-- stripped fenced code block: yaml -->

**Option 2:**
This option entails creating a new top-level pre-caching field in the CGU named `preCache`. This field
comprises of the `enable` and the `configRef` fields. The former is used to enable/disable pre-caching
whereas the latter is used to specify the reference to the pre-caching configuration CR.

The intention behind this approach is to contain all pre-caching fields under one object and to
deprecate the current `preCaching` field. Note that if users continue to use the original `preCaching`
field - that is fine. However, the `precache.enable` field will take precedence (or cause failure if
both are set and not the same).

This approach is shown below.

<!-- stripped fenced code block: yaml -->

**Option 3:**
This option offers an idealistic solution by avoiding multiple pre-caching fields by modifying the
top-level `preCaching` CGU spec field from boolean type to map type. This approach requires transitioning
to a new `apiVersion` for the CGU CR (`apiVersion: ran.openshift.io/v2alpha1`).

A major drawback to this approach is that we need to ensure backwards compatibility, in other words, we
would need to support both `/v1alpha1` and `/v2alpha1` versions of the API simultaneously.

This approach is shown below.

<!-- stripped fenced code block: yaml -->

**Selected approach for referencing the PreCachingConfig CR in the CGU:**

The selected approach for referencing the pre-caching config CR in the CGU is Option 1. The primary motivation factor
driving the selection is the retro-compatibility offered by Option 1. Furthermore, any new pre-caching configurations 
can easily be added to the `PreCachingConfig` CR, thus there is no forecasted additional pre-caching fields to the CGU.


### Suggested Changes

The following changes are suggested:

1. Create the pre-caching configuration custom resource definition (CRD) as shown below.
  The TALM Makefile needs to be updated in order to install the new CRD into the OpenShift
  cluster using `kubectl apply -f`.
<!-- stripped fenced code block: yaml -->

2. Add a new field `PreCachingConfigRef` to the `ClusterGroupUpgradeSpec` struct in
`cluster-group-upgrades-operator/api/v1alpha1/clustergroupupgrade_types.go` as shown below.
<!-- stripped fenced code block: golang -->

3. Add the `AdditionalImages` and `SpaceRequired` fields to the `PrecachingSpec` struct
  `cluster-group-upgrades-operator/api/v1alpha1/clustergroupupgrade_types.go` as shown below.
<!-- stripped fenced code block: golang -->

4. Apply the operator-sdk tool to update the TALM custom resource definitions, manifests and auto-generated
   golang source files.

5. Extend the `controllers/managedClusterResources.go` and
`cluster-group-upgrades-operator/controllers/precache.go` source files to handle reconciliation
control logic pertaining to the newly added pre-caching config CR. This entails handling the override
logic as well. Both overriding sources of inputs need to be supported, i.e. the new `PreCachingConfig`
CR as well as the current `cluster-group-upgrade-overrides` ConfigMap. Precedence is to be given to
the `PreCachingConfig` CR. If the ConfigMap is specified and is consumed, TALM will emit a log message
warning the user that its use is now deprecated.

6. Extend the pre-cache template source file `controllers/templates/precache-templates.go` with the
`additionalImages` field.

7. Refactor the `cluster-group-upgrades-operator/pre-cache/precache.sh` and
`cluster-group-upgrades-operator/pre-cache/pull` scripts to check for additional user images and pull
them accordingly. The existing image pulling functionality should be refactored in order to promote
code re-usability and avoid code duplication.
Note that an attempt will be made to enhance the logging capability of the scripts to provide the user
with an appropriate message to aid investigations pertaining to images that could not be successfully
pre-cached.

### Workflow Description

The general workflow for pre-caching user specified images is as follows:
`ClusterSelected -> Validated -> PrecacheSpecValid -> PrecachingSucceeded -> <TALM workflow continues>`.

Detailed user and workflow steps are outlined below.

1. User creates a CGU CR with the `preCaching` field set to `true` and prepares a pre-caching config
CR with the list of additional images to be pre-cached as shown in the examples below. Note that the
CR should be created concurrently with the CGU or should exist prior to the creation of it. Furthermore,
the pre-caching config CR should be created in a namespace accessible to the hub cluster.
   - Example CGU CR:
<!-- stripped fenced code block: yaml -->
   - Example pre-caching config CR:
<!-- stripped fenced code block: yaml -->

2. The CGU CR is validated (a function verifies if the clusters specified in the CR are part of a `ManagedCluster`
object).

3. If the validation succeeds, the pre-caching reconciliation commences. Note that all the sites are
pre-cached concurrently (consistent with existing behavior).

4. The pre-cache `spec` is then validated by checking if the managed policies exist. A valid `spec` will
result in the following condition:
    ```text
    Type:    "PrecacheSpecValid",
    Status:  True,
    Reason:  "PrecacheSpecIsWellFormed",
    Message: "Precaching spec is valid and consistent"
    ```

5. Pre-caching commences upon successful validation of the pre-cache `spec`. The following conditions are
set while pre-caching is in-progress:
    ```text
    Type:    "PrecachingSuceeded",
    Status:  False,
    Reason:  "InProgress",
    Message: "Precaching is required and not done"
    ```

6. After the pre-caching job is successfully completed, the following condition is set:
    ```text
    Type:    "PrecachingSuceeded",
    Status:  True,
    Reason:  "PrecachingCompleted",
    Message: "Precaching is completed for all clusters"
    ```

7. The remaining workflow for TALM continues as normal from here onwards.

#### Variation [optional]

N/A

### API Extensions

The following API exensions to TALM will be made:
- A new CRD `PreCachingConfig` will be added to specify the pre-caching configurations.
- The current CGU CRD will be modified with the addition of a new field `PreCachingConfigRef` that will reference the 
corresponding `PreCachingConfig` CR.

### Implementation Details/Notes/Constraints [optional]

The implementation details pertaining to this EP are briefly summarized below:
- The additional image spec must be fully qualified (full registry) with the SHA.
- This EP targets pre-caching for SNO-based architectures only.
- The EP applies the existing status conditions and reasons, i.e. no changes are made to the
current TALM pre-caching status conditions/reasons.
- It is the responsibility of the user to ensure sufficient storage capacity proportional to the number
of additional images to be pre-cached. This is to be specified using the `spaceRequired` field in
the `PreCachingConfig` CR.


### Risks and Mitigations

N/A

### Benefits

The benefits of the proposed approach are as follows:
- The solution extends the current pre-caching functionality offered by TALM in a unified approach.
- The proposal presents an opportunity to refactor TALM's pre-caching functionality to be more
future-proof.
- No additional operators need to be installed (apart from TALM) and thus compute requirements
remain unchanged.

### Drawbacks

A drawback of the proposed approach is that TALM now accepts the additional workload of pre-caching
user specific images. There might be an additional, insignificant cost of parsing the additional images
field on the hub cluster. However, it is anticipated that this not an expensive computational operation.

## Design Details

### Open Questions [optional]

The following questions are open to discussion:
- What is the most effective way of checking the pre-caching disk usage after pulling the images? This
question aims at addressing the potential kubelet garbage collection problem that can result in the deletion
of the pre-cached images (thus nullifying the entire pre-caching job).
  - One approach is to compare the space consumed by the pre-cached images and the `spaceRequired` parameter,
  and if the latter is smaller, treat the pre-caching job as a failure.
  - Another approach consists of retrieving the `imageGCHighThresholdPercent` parameter from the cluster and
  checking the disk usage post pre-caching (i.e validate that the pre-caching job will not trigger the garbage
  collection). In the current implementation of CRI-O, the images that are pulled do not necessarily reflect
  the latest timestamps and can still be deleted. Nonetheless, this approach will at least indicate whether
  GC will run and report the status in the CGU CR accordingly.


### Test Plan

N/A

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

TALM pre-caching supports versions across `N-2`, i.e. TALM version `N` can pre-cache on a cluster that is
up to version `N-2`.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A.

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

Create a new daemonset with a dedicated user specific workload image pre-caching operator.

**Pros:**
- User specific application images are pre-cached separate from TALM.

**Cons:**
- Additional operator needs to be installed. This impacts the compute requirements for the user as there
are significant overhead costs of deploying operators (logging, performance monitoring, liveness probes, etc).

## Infrastructure Needed [optional]
N/A
