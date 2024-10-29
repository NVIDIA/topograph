# Topograph with Kubernetes

In Kubernetes, Topograph performs two main actions:

- Creates a ConfigMap containing the topology information.
- Applies node labels that define the node’s position within the cloud topology. For instance, if a node connects to switch S1, which connects to switch S2, and then to switch S3, Topograph will label the node with the following:
  - `topology.kubernetes.io/network-level-1: S1`
  - `topology.kubernetes.io/network-level-2: S2`
  - `topology.kubernetes.io/network-level-3: S3`

## Configuration and Deployment
TBD

## Validation and Testing
TBD
