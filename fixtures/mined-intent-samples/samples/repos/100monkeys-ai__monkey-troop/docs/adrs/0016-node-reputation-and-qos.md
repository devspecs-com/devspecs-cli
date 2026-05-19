# 16. Node Reputation and Quality of Service (QoS)

Date: 2026-03-14

## Status

Accepted

## Context

In an open, distributed network where anyone can join as a worker, there is a risk of low-quality, unreliable, or even malicious nodes. We need a way to incentivize high performance and reliability while discouraging poor behavior.

## Decision

We propose implementing a **Node Reputation and Quality of Service (QoS)** system.

1. **Reputation Scoring**: The coordinator will maintain a reputation score for each worker node based on several factors:
    * **Availability**: Uptime and consistent heartbeat participation.
    * **Reliability**: Success rate of inference requests and timely submission of receipts.
    * **Performance**: Historical throughput (tokens/second) relative to the node's PoH hardware multiplier.
    * **Accuracy**: Correctness of inference results (verified periodically via "consensus checks" where multiple nodes process the same prompt).
2. **Reputation-Based Discovery**: The coordinator's `/peers` endpoint will prioritize nodes with higher reputation scores.
3. **Tiered Access**: High-reputation nodes may be granted access to more demanding or higher-value inference tasks.
4. **Penalties**: Nodes with low reputation scores will be penalized, possibly through reduced visibility in discovery or temporary suspension from the network.

## Consequences

* **Network Quality**: The reputation system will drive workers to maintain high levels of availability, performance, and accuracy.
* **Trust**: Clients can be more confident in the quality of the nodes they connect to.
* **Incentives**: Providers are incentivized to invest in stable and performant hardware and infrastructure.
* **System Integrity**: The system can more effectively identify and remove problematic or malicious nodes from the network.
* **Complexity**: Implementing a fair and robust reputation system is challenging and requires careful consideration of various edge cases.
* **Privacy**: Reputation scoring must be done carefully to avoid leaking sensitive information about the tasks a node has processed.
