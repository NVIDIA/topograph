# Topograph with Kubernetes

Topograph is a tool designed to enhance scheduling decisions in Kubernetes clusters by leveraging network topology information.

### Overview

Topograph's primary objective is to assist the Kubernetes scheduler in making intelligent pod placement decisions based on the cluster's network topology. It achieves this by:

1. Interacting with Cloud Service Providers (CSPs)
2. Extracting cluster topology information
3. Updating the Kubernetes environment with this topology data

### Current Functionality

Topograph performs the following key actions:

1. **ConfigMap Creation**: Generates a ConfigMap containing topology information. This ConfigMap is not currently utilized but serves as an example for potential future integration with the scheduler or other systems.

2. **Node Labeling**: Applies labels to nodes that define their position within the cloud topology. For example, if a node connects to switch S1, which connects to switch S2, and then to switch S3, Topograph will apply the following labels to the node:

   ```
   topology.kubernetes.io/network-level-1: S1
   topology.kubernetes.io/network-level-2: S2
   topology.kubernetes.io/network-level-3: S3
   ```

### Use of Topograph

While there is currently no fully network-aware scheduler capable of optimally placing groups of pods based on network considerations, Topograph serves as a stepping stone toward developing such a scheduler.

Topograph can be used in conjunction with Kubernetes' existing PodAffinity feature.
This combination enhances pod distribution based on network topology information.

The following excerpt describes a Kubernetes object specification for a cluster with a three-tier network switch hierarchy. The goal is to improve inter-pod communication by assigning pods to nodes within
closer network proximity.

```yaml
    affinity:
      podAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 70
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - myapp
              topologyKey: topology.kubernetes.io/network-level-2
          - weight: 90
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - myapp
              topologyKey: topology.kubernetes.io/network-level-1
```
Pods are prioritized to be placed on nodes sharing the label `topology.kubernetes.io/network-level-1`.
These nodes are connected to the same network switch, ensuring the lowest latency for communication.

Nodes with the label `topology.kubernetes.io/network-level-2` are next in priority.
Pods on these nodes will still be relatively close, but with slightly higher latency.

In the three-tier network, all nodes will share the same `topology.kubernetes.io/network-level-3` label,
so it doesn’t need to be included in pod affinity settings.

Since the default Kubernetes scheduler places one pod at a time, the placement may vary depending on where
the first pod is placed. As a result, each scheduling decision might not be globally optimal.
However, by aligning pod placement with network-aware labels, we can significantly improve inter-pod
communication efficiency within the limitations of the scheduler.

## Configuration and Deployment
TBD

## Validation and Testing
TBD
