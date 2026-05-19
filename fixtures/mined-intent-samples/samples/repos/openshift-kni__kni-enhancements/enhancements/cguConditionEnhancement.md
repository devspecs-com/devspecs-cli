# Cluster Group Upgrade (CGU) conditions and their reason/messages enhancement

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Documentation is updated in the upstream
- [ ] Documentation is updated in the downstream


## Summary

Cluster Group upgrade (CGU) returns multiple condition type under status field to provide the status update for backup, precaching and upgrade jobs via reason and messages associated to the condition. 
This enhancement looks to improve the condition and its related reason and messages to the granular level for better understanding/reporting the underlying workflow. 

Bugs that led to this enhancement proposal: [OCPBUGSM-39409](https://issues.redhat.com/browse/OCPBUGSM-39409)  and [OCPBUGSM-39588](https://issues.redhat.com/browse/OCPBUGSM-39588) 

## Motivation

To understand the motivation for the enhancement, first we have to understand the current state of the CGU conditions that is returned under **.status.condition** field. 

Currently, the upgrade workflow only returns a **READY** condition type related to Upgrade with various reason and messages and the timestamp is only updated when the codition status is changed/finished keeping user/dev unaware of at what stage is CGU progressing with the validation.


### Goals

- Enhance the CGU status *"READY"* condition to granular level to capture the underlying upgrade workflow/stages to better understand the succesful/failure steps.
- Re-arrange the present validation and functionalities into granular level 

### Non-Goals

- Avoid introducing new functionalities and verification related to conditions.

### Background

The condition is found under the CR status field which indicates the CR activity/progress status using Types. It uses meta/v1 api package. In TALM, there are types related to backup, precaching and upgrade. The status can be *true/false/unknown*

#### <a id="upgrade"></a>**Cluster Upgrade:**

Only 1 condition type “Ready” is used with multiple reason and custome messages like below:  

<!-- stripped fenced code block: yaml -->

  **Current Mapping between Status, Reason and Message:**

  | Status| Reason| Message |
  |-------|-------|---------|
  True| UpgradeCompleted|1. The ClusterGroupUpgrade CR has all clusters already compliant with the specified managed policies .<br>2. The ClusterGroupUpgrade CR has all clusters compliant with all the managed policies. <br>3. The ClusterGroupUpgrade CR has all upgrade policies compliant|
  False| UpgradeCannotStart| 1. The ClusterGroupUpgrade CR has blocking CRs that are missing .<br>2. The ClusterGroupUpgrade CR is blocked by other CRs that have not yet completed .<br>3. The ClusterGroupUpgrade CR has managed policies that are missing|
  False| UpgradeNotCompleted| The ClusterGroupUpgrade CR has upgrade policies that are still non compliant|
  False| UpgradeNotStarted| The ClusterGroupUpgrade CR is not enabled|
  False|UpgradeTimedOut| The ClusterGroupUpgrade CR policies are taking too long to complete
  False| ``*PrecachingRequired (This reason is set, when precaching is true but not started yet)``| Precaching is not completed (required)|

  ```diff
  - When precaching field is set to true, the reason is updated under `READY` condition, although upgrade hasn’t started.
  ```

<!-- stripped fenced code block: yaml -->

#### <a id="precaching"></a>**PreCaching:** 

There are 2 condition types used for precaching, they are:

- PrecacheSpecValid
- PrecachingDone

 
<!-- stripped fenced code block: yaml -->

##### <a id="precachingSpec"></a>**Current Mapping between Status, Reason and Message:**

  | Type | Status| Reason| Message |
  |------|-------|---------|--------|
  PrecachingDone| True| PrecachingCompleted| Precaching is completed|
  PrecachingDone|False| PrecachingNotDone| Precaching is required and not done|
  PrecacheSpecValid| True| PrecacheSpecIsWellFormed| Pre-caching spec is valid and consistent|
  PrecacheSpecValid| False| NotAllManagedPoliciesExist| The ClusterGroupUpgrade CR has managed policies that are missing|
  PrecacheSpecValid| False| `PrecacheSpecIsIncomplete`|1. Inconsistent precaching configuration: olm index provided, but no packages.<br>2. Inconsistent precaching configuration: no software spec provided|
  PrecacheSpecValid|False|`PlatformImageConflict`| Platform image must be set once, but x and y were given|
  PrecacheSpecValid| False| `PlatformImageInvalid`| Error message is returned|


  ```diff
  - currently PrecacheSpecIsIncomplete, PlatformImageConflict,PlatformImageInvalid reason is only checked during precaching validation and not checked if precaching is set to false. Either way, this checks needs to be done irrespective to precaching being true or false, because failing to check those will also make a upgrade to be failed.
  ```

#### <a id="backup"></a>**Backup:** 

Currently the backup job has only one condition type.

 
<!-- stripped fenced code block: yaml -->


  **Current Mapping between Status, Reason and Message:**

  | Status| Reason| Message |
  |-------|---------|-------|
  True| BackupCompleted| Backup is completed|
  False| BackupNotDone| Backup is required and not done|


### Current Workflow and limitations

The idea is to write down the current workflow, find opportunities where we can break the ready condition to a bit more granular conditions and highlighting limitations.

The below workflow description considers both backup and precaching is set to true in the CGU

1. Validate the CR, here the function checks if the list of clusters in the CR is part of managedcluster object. 
> There can be a condition for CR validation related to cluster, which fixes the bug [OCPBUGSM-39409](https://issues.redhat.com/browse/OCPBUGSM-39409)

2. When the backup is set to true, it will block any other operation until it is successful, if it is failed, the upgrade procedure will be halted. So the condition type BackupDone either evolves to true -> BackupCompleted, or false -> BackupNotDone
3. Afterwards, reconciling to precache starts by setting the below conditions:
 
<!-- stripped fenced code block: yaml -->

4. PrecacheSpecvalidation with different status, reason and message can be found in the [Precaching subsection](#precaching). The Spec validation is done via checking if managed policies exist. Once the spec is validated below condition type is set:

<!-- stripped fenced code block: yaml -->

5. After the spec validation, when precache starts, it sets the two below condition while pre-cache is on-going and evolves if precaching is successful:

<!-- stripped fenced code block: yaml -->

6. If precache is not set to true, the steps 3,4 and 5 aren’t executed and the above mentioned condition types do not appear. In that case, the `Ready` condition is set as:

<!-- stripped fenced code block: yaml -->

7. Upgrade actually proceeds by finding if type Ready has been set with either **UpgradeNotStarted** or **UpgradeCannotStart**

8. The upgrade task starts by checking if *<Spec.Enable is True>*. If yes then following verification takes place:
  
   * Checks if the managed policies exist, if not it sets the condition:

<!-- stripped fenced code block: yaml -->

    > The caveat here is that, when precache is set to true, we check twice if the managed policies exist with *PrecacheSpecValid* and *Ready* condition type. <br> May be it can be separated from both and can become a generic condition -> SpecValidation

    * Afterwards it checks if blocking CR missing or not being complete and on failure **UpgradeCannotStart** Reason is set for `Ready` condition.
    > It seems the upgrade can’t start for 2 reasons, <br> 1) If managed policies are missing, <br> 2) If blocking CRs are missing or not complete. Do we have to have a separate condition type for blocking CR verification?

      
    * there are 2 actions that can be defined under the Spec.Actions in the CGU -> beforeEnable and afterCompletion. `beforeEnable` defines the action to be done before starting an upgrade by adding or deleting cluster label. Once it is done, it sets the **UpgradeNotCompleted** Reason under `READY` condition:

<!-- stripped fenced code block: yaml -->

      > May be this verification can move to pre-upgrade validation

9. The **UpgradeNotCompleted** and **UpgradeCompleted** is differentiated by checking whether the current batch is equal to the length of the remediation plan.

### User Stories

- As an end user/dev, I would like to understand which steps are finished/succeeded/failed during cluster upgrade.

## Recommendations

This section highlights the recommendation on condition types from kubernetes community and what other operators are following the design pattern in general.

- Article provided by @ricardo: [Conditions in Kubernetes Controller](https://maelvls.dev/kubernetes-conditions/)
- [Cert-manager: Automatically provision and manage TLS certificates in Kubernetes](https://github.com/cert-manager/cert-manager) uses the single Ready condition which matches to the current TALM condition type
- [Kubernetes recommendation on Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)
- Posted a question in slack channel : #forum-apiserver for any existing design guidelines. Still waiting for some feedback.
- **Openshift-apiserver, kube-apiserver, machine-config, node-tuning, config-operator** have below conditions:

<!-- stripped fenced code block: yaml -->

- **Network operator** has below condition:

<!-- stripped fenced code block: yaml -->

## Proposal

This section proposes minimalistic changes without breaking the current design pattern. The below proposal/recommendation consists of more granular `status.conditions`  based on all the verification we already have in TALM.

**Considering**: backup,precaching is true and enable for upgrade is true as well. 

### Workflow Description

1. Condition type related to cluster existing in the managedcluster. The type could reflect on the existence of the targeted cluster to be present in the managedcluster. 

<!-- stripped fenced code block: yaml -->

2. a) Spec validation for the `managed policies`. Currently it gets checked both during pre-caching and for upgrade. This check can be done before starting an upgrade, irrespective of pre-caching being true or false. b) Platform related verification. 
Point a) and b) can be merged into **Validated** (previously proposed as PreUpgradeValidation/Upgradeable) where we check managed policies existence, conformity of platform image etc. to understand if upgrade can proceed.

<!-- stripped fenced code block: yaml -->

3. Condition type: **PrecacheSpecValid** and **PrecachingDone**(it will become **PrecachingSucceeded**) which is already in the current design. For **PrecacheSpecValid**, we may omit the highlighted Reason from the [current spec](#precachingSpec)

4. Condition type: **BackupDone** becomes **BackupSucceeded**

5. Checking the blocking CRs status, if there is any issue UpgradeCannotStart reason is reported. If CRs are not blocked the upgrade can start. Also, adding and deleting labels to the cluster can be performed here. Proposing new Condition type: Progressing

<!-- stripped fenced code block: yaml -->

  6. The **UpgradeCompleted** and **UpgradeTimedOut** Reason can be moved to condition type: Succeeded

<!-- stripped fenced code block: yaml -->

### Workflow summary

- When backup and Precaching is true:
  * ClusterSelected -> Validated ->  PrecacheSpecValid -> PrecachingSucceeded -> BackupSucceeded -> Progressing -> Succeeded
 

- When backup and precaching is false:
  * ClusterSelected -> Validated -> Progressing -> Succeeded

### Mapping between Type, Status, Reason and Message

Based on the workflow that is described above, the summarized conditions,Reason and messages are grouped in a table below:

if backup and/or precaching is false, the related types will not appear in `.status.conditions`.

  | Type | Status| Reason| Message |
  |------|-------|---------|--------|
  `ClustersSelected`| True| Completed| All selected clusters are valid for upgrade| 
  | |False | NotFound/NotPresent| Unable to select clusters: error message |
  `Validated` | True | Completed| Completed validation |
  | | False | NotAllManagedPoliciesExist| Missing managed policies: policyList,  invalid managed policies: policyList |
  | | False | InvalidPlatformImage | Precache spec is incomplete |
  `PrecacheSpecValid` | True | PrecacheSpecIsWellFormed | Precaching spec is valid and consistent |
  | | False | InvalidPlatformImage| Precaching spec is incomplete |
  `PrecachingSucceeded` | True | Completed | Precaching is completed for all clusters|
  | | True | PartiallyDone | Precaching failed for x clusters | 
  | | False | InProgress | Precaching is not completed | 
  | | False | InProgress | Precaching is in progress for x clusters | 
  | | False | Failed | Precaching failed for all clusters |
  `BackupSucceeded` | True | Completed | Backup is completed for all clusters|
  | | True | PartiallyDone | Backup failed for x clusters |
  | | False | InProgress | Backup is in progress for x clusters|
  | | False | Failed | Backup failed for all the clusters |
  `Progressing`| True | InProgress| Remediating non-compliant policies|
  | | False | Completed | All clusters are compliant with all the managed policies |
  | | False | NotStarted | The Cluster backup is in progress |
  | | False | NotEnabled| The ClusterGroupUpgrade CR is not enabled |
  | | False | MissingBlockingCR | The ClusterGroupUpgrade CR has blocking CRs that are missing |
  | | False | IncompleteBlockingCR | The ClusterGroupUpgrade CR is blocked by other CRs that have not yet completed| 
  `Succeeded`| True | Completed| All clusters compliant with the specified managed policies |
  | | False | TimedOut | Policy remediation took too long |


- If `ClustersSelected` is false, it should block the subsequent states, meaning we do not proceed to backup, precaching, upgrade 
- When Upgrade starts, Progressing initiates as True, when Succeeded becomes true or false, Progressing turns to false with reason Completed. 
- Succeeded state should only appear when Progressing is false and completed
- For user/admin to verify whether the upgrade cycle is succeeded or timed out, they must rely on type `Succeeded` to be either true or false.
  
### Condition example

- All the condition type must not appear unless the current state transition to next next state, meaning, when `Validated` condition type is being verified, the next state must not appear in the conditions. When Validated type is completed, `BackupSucceeded` type should appear and so on.

<!-- stripped fenced code block: yaml -->

### Risks and Mitigations

The idea of this enhancement is to either remove the `READY` condition and/or break the condition into more granular level. If any tool or operator relies or consumes the `READY` condition, it will be invalid condition or response.

## Implementation choices

Backup will be only initiated after enable is set to `true` when "`backup:true`" in the CGU . In case of chained CGU cases, backup will be started after blocking CGUs are completed. This implemetation choice does not require a new implementation, rather backup implementation can be moved to the approprite places in the code block. 

### API Extensions

N/A

### Drawbacks

N/A
## Design Details

N/A

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

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

N/A
