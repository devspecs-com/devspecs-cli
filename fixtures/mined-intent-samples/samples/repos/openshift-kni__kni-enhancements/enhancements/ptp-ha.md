# PTP multi-nic HA Support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Multi-nic support for PTP operator [1] seems to be already built-in. In
enterprise telco environments, there's often a need to load balance two
different signals from different PTP IEEE 1588-2008 sources.

PTP uses best master clock (BMC) algorithm to perform a selection of the
candidate clock based on a series of parameters such as accuracy, quality,
stratum and uuid. The HA support should also consider these ones and perform
the change when needed.

In Telco environments, where an specific profile, such as G.8275.1[] is needed,
ABMC (Alternate BMC is required).

## Scenarios

### Multiple NICs

This case assumes that over the that the Virtual Distributed Unit (vDU) hosts
several NICs supporting Boundary Clock Function. The synchronization source is
selected by *ptp4l*

On Openshift 4.10 the PTP operator supports running multiple NICs with the
following dual nic with two ptp4l configurations, but no HA on failure.

<!-- stripped fenced code block: text -->

HA on failure would require several NICs connection at the Remote Unit (RU)
level as well, so they would be able to load balance between the two boundary
clocks within the boundary clock.

### Multiple interfaces on one NIC

```text
┌─────────────────────────────────────┐
│                                     │
│                                     │
│      Grand Master Clock/GPS source  │
│                                     │
│                                     │
│                                     │
│                                     │
└─────▲─────────────────────▲─────────┘
      │                     │
      │                     │
      │                     │
      │                     │
┌─────┼─────────────────────┼─────────┐
│     │        vDU          │         │
│  ┌──┴─────┐            ┌──┴────┐    │
│  │        │            │       │    │
│  │ NIC 1  │            │ NIC 2 │    │
│  │        │            │       │    │
│  └▲───────┘            └──▲────┘    │
│   │                       │         │
└───┼───────────────────────┼─────────┘
    │                       │
    │     ┌─────────────────┘
    │     │
    │     │
 ┌─┬┼┬───┬┼┬──┐
 │ │┼│ c|│┼│  │
 │ └─----└─┘  │
 │Multiple If │
 │            │
 │            │
 │            │
 │            │
 │            │
 │            │
 │  RU 0      │
 │            │
 └────────────┘
```

Some NICs would support that but some others (i.e Collumbiaville) would just have one single PTP.
In such scenario, this card itself would directoy perform the sync without any additional requirement from the operator
or



### How to detect when a clock is off sync

Whenever a clock goes off sync, the operator should be aware of this event and react accordingly to avoid loss of sync.

This could be detected by using the offset from the slave interface via:

* logs

* metrics from prometheus (again offset)

* clock change events, such as clock class event.

## Motivation

### Goals

- Support multiple PTP sources within the PTP operator
- Add an additional field in the CR regarding stratum priority

### Non-Goals

- This EP doesn't consider CPU pinning assigned to the different processes.

## Proposal

### Reconcile Loop

The PTP Operator's reconcile loop would also take care of checking whether the
PTP clock has failed, should the HA field be on the CRD.

Currently, the logic itself is on the `ptpdaemon`. The `ph2sys` process serves a restart mechanism for this.

The posible options for handle this would be to either kill `ph2sys` extending the current behavior or write a full
fledged controller.

An initial implementation will use a linux-ptpdaemon configmap, such as:

<!-- stripped fenced code block: json -->

```console
$ kubectl get configmap linuxptp-configmap -o yaml -n openshift-ptp
apiVersion: v1
data:
  node.example.com: |
    {"interface":"ens786f1", "ptp4lOpts":"-s -2", "phc2sysOpts":"-a -r"}
kind: ConfigMap
metadata:
  creationTimestamp: "2019-10-10T09:03:39Z"
  name: linuxptp-configmap
  namespace: openshift-ptp
  resourceVersion: "2323998"
  selfLink: /api/v1/namespaces/openshift-ptp/configmaps/linuxptp-configmap
  uid: 40c031f4-e09d-40a8-b081-92c3b8c0accb
```

Will be used to draft and complete the HA logic. The controller and and API will be added later in the operator.
Initial work will be  only in LinuxPTPdaemon, since the PTP operator is just a dum,u controller which creates configmap
and deploys daemonsets. Most of the HA behavior will be implemented by overriding via configmaps.

#### What does failure mean?

Failure would mean that the sync with the upstream clock is lost, and that the
local ptp daemon is on freerun. It would have its sync lost bit by bit, even
though it may still be working for some time.

Once such situation is identified, We would need to kill the phc2sys daemon.
The secondary nic will configured with its `phc2sys` crd config set to zero
so it doesn't start with any config.

Besides the discussed above scenarios, we may want to also consider the change in clock class. We'd be covering the
change of clock class, as it may be changing the precision on this. i.e. (GPS - Clock Class Event)

### HA events

The HA switchover events will be, not limited to:

* Grace period to avoid switchover/Declaring a fault.
* Clock faulty if failure persist (i.e. 1 second)
* HA event

We should include a graceover period to avoid constant flip-flopping of the HA clock solution.

### Active/Passive for NICs
The Active/Passive scenario for the NICs is outside of the scope of this proposal.

### Active/Active for NICs
In this scenario `phc2sys` selects which one to run based on algorithm (link status/clock status)

### Workflow Description

### API Extensions

### User Stories

## Design Details

The linuxptp package includes ptp4l and phc2sys programs for clock
synchronization. The PTP Operator runs these two programs into a
linuxptp-daemon pod on each node which belongs to the cluster.

### ptp4l

It implements the PTP boundary clock and ordinary clock. It synchronizes the
PTP hardware clock (PHC) from the NIC to the source clock with hardware time
stamping and synchronizes the system clock to the source clock with *software*
time stamping.

### phc2sys

phc2sys is used for hardware time stamping to synchronize the system clock to
the PTP hardware clock on the network interface controller (NIC).

### pmc

It's an utility to configure ptp4l in runtime. It's also available within the
linuxptp-daemon pod.

## Alternatives

Using `boundary_clock_jbod` option within the profiles allows a node acting as
a BC using two different NICs to have their PHCs synced via `phc2sys`. That
said, this would go over the max time error (30 ns) due to the increased
offset of having such synchronization via software (as it used `phc2sys`).

This error (Max. Absolute Time Error (max|TE|)) would only apply to the
fronthaul transport network.
### Telecom profiles

Following up with what was described in the above section,  we would need to
decided whether to use one of these profiles:

* G.8275.1

The G.8275.1 profile is designed to provide the most accurate time
synchronization solution within the constraints of +-1.5us from the Primary
Reference Time Clock (PRTC)

* G.8275.2

As G.8275.1 implies that every node has to support aBC with a certain level of
accuracy and this is not always possible. G.8275 allows PTP packets to be
distributed using IPv4/IPv6 unicast rather L2 multicast as used in G.8275.1

This enables PTP packets to be distributed over non-PTP networks and across L"
boundaries.

As each node in the network does not have a physical clock, this is referred as
partial timing support. Accuracy is lower but still acceptable.

|                	|         G.8275.1 (Full Timing Support)         	|                   G.8752.2 (Partial Timing Support)                  	|
|----------------	|:----------------------------------------------:	|:--------------------------------------------------------------------:	|
| Transport      	| PTP over Ethernet Multicast                    	| PTP over IPv4 or IPv6 Unicast. IP QoS with DiffServ for sync packets 	|
| BMCA Algorithm 	| Alternate BMCA (A-BMCA), as specified by ITU-T 	| Alternate BMCA (A-BMCA), as specified by ITU-T                       	|

### ptpHAConfig to set up boundary clock using multiple interface with HA

There's a new API introduced for PtpHAConfig, which initially would behave like this:


<!-- stripped fenced code block: go -->

Taking the original model for setting up a boundary clock using multiple
interfaces it adds a few additional fields:

* EnableHA: enables HA config
* PreferredPrimary: which interface use as the main one
* HeartBeatTimeOut: timeout to do the HA handover

As stated in other section, this would be coupled to a new ConfigMap in the ptp4linux daemon repository, which would
trigger the needed config for `phc2sys`

### Risks and Mitigations

### Drawbacks

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History
---
[1] https://github.com/openshift/ptp-operator
[2] G.8275.1 : Precision time protocol telecom profile for phase/time synchronization with full timing support from the network
    https://www.itu.int/rec/T-REC-G.8275.1-202003-I/en
[3] G.8275.2 : Precision time protocol telecom profile for time/phase synchronization with partial timing support from the network
    https://www.itu.int/rec/T-REC-G.8275.2-202003-I/en
[4] https://github.com/openshift/linuxptp-daemon
