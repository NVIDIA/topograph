# Topograph NFD Engine

The `nfd` engine publishes Topograph topology through
[Node Feature Discovery](https://kubernetes-sigs.github.io/node-feature-discovery/)
custom resources instead of writing Kubernetes node labels directly.

It creates:

- one `NodeFeature` per topology node, carrying Topograph topology as
  `spec.features.attributes.topograph.network.elements`
- one `NodeFeatureGroup` per distinct accelerator, leaf, spine, and core value

NFD master evaluates those features and writes matching nodes to
`NodeFeatureGroup.status.nodes`.

## When to Use

Use `engine: nfd` only when a downstream component already consumes NFD
`NodeFeatureGroup` objects. For native Kubernetes scheduling with
`podAffinity.topologyKey`, use the [`k8s`](./k8s.md) engine instead.

NFD `NodeFeatureGroup` is an alpha API and is disabled by default in NFD master
at the time of writing. Enable the matching NFD feature gate before using this
engine. See the NFD
[custom resource documentation](https://kubernetes-sigs.github.io/node-feature-discovery/master/usage/custom-resources.html#nodefeaturegroup)
for the current upstream state.

## Configuration

```yaml
provider:
  name: infiniband-k8s
engine:
  name: nfd
  params:
    nodeSelector:
      nvidia.com/gpu.present: "true"
    cleanup: true
```

Parameters:

| Parameter | Required | Default | Description |
|---|---:|---|---|
| `nodeSelector` | No | all nodes | Limits the Kubernetes nodes used as provider input. Same meaning as the `k8s` engine selector. |
| `cleanup` | No | `true` | Deletes stale Topograph-managed `NodeFeature` and `NodeFeatureGroup` objects that are no longer present in the generated topology. |
| `namespace` | No | empty | Namespace for namespaced NFD CRs. Set it to the NFD master namespace because NFD resolves `NodeFeatureGroup` updates there. |

## Generated Objects

For a node on `leaf-12`, the engine writes a `NodeFeature` like:

```yaml
apiVersion: nfd.k8s-sigs.io/v1alpha1
kind: NodeFeature
metadata:
  name: topograph-node-node-a-...
  labels:
    nfd.node.kubernetes.io/node-name: node-a
    app.kubernetes.io/managed-by: topograph
    topograph.nvidia.com/engine: nfd
spec:
  features:
    attributes:
      system.name:
        elements:
          nodename: node-a
      topograph.network:
        elements:
          accelerator: nvl3
          leaf: leaf-12
          spine: spine-2
          core: core-1
```

For each distinct value, it writes a matching `NodeFeatureGroup`:

```yaml
apiVersion: nfd.k8s-sigs.io/v1alpha1
kind: NodeFeatureGroup
metadata:
  name: topograph-leaf-leaf-12-...
  labels:
    app.kubernetes.io/managed-by: topograph
    topograph.nvidia.com/engine: nfd
    topograph.nvidia.com/group-type: leaf
  annotations:
    topograph.nvidia.com/label-key: network.topology.nvidia.com/leaf
    topograph.nvidia.com/label-value: leaf-12
spec:
  featureGroupRules:
    - name: leaf equals leaf-12
      matchFeatures:
        - feature: topograph.network
          matchExpressions:
            leaf:
              op: In
              value: ["leaf-12"]
```

Topograph includes `system.name.elements.nodename` so NFD can populate group
membership even when an NFD worker does not run on the node, as with simulated
KWOK nodes. Topograph does not write `status.nodes`; NFD owns status updates.

If a Kubernetes node already has `nvidia.com/gpu.clique`, the engine uses that
label's value as the authoritative accelerator attribute instead of the value
derived from the provider graph. The matching `NodeFeatureGroup` records
`nvidia.com/gpu.clique` as its source label key. Leaf, spine, and core topology
attributes are still published.

## Caveats

`NodeFeatureGroup` objects enumerate topology groups. A scheduler that wants to
place a workload within one leaf switch must still decide which leaf group to
use. This is different from native pod affinity, where the scheduler can compare
candidate nodes using a single `topologyKey`.

Large clusters may create many CRs and large `status.nodes` arrays. The design
notes in [`docs/design/nfd-engine-sdd.md`](../design/nfd-engine-sdd.md) include
the storage estimate and possible future NFD improvements such as group unions
and compressed node-name ranges.
