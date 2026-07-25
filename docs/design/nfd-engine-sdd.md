# Software Design Document: NFD Engine

## Status

Implemented.

## Summary

Add an experimental `nfd` engine that converts Topograph's canonical
`topology.Graph` into Node Feature Discovery (NFD) `NodeFeatureGroup` objects.
The engine creates one group for each distinct topology label value, such as one
group for each distinct fabric-tier or accelerator value.

This should not replace the current `k8s` engine. The `k8s` engine writes node
labels that can be consumed by native Kubernetes affinity and topology-aware
schedulers. The `nfd` engine would publish equivalent group membership through
NFD CRs for consumers that already watch NFD.

## Background

Topograph already maps topology into four Kubernetes label dimensions:

- `network.topology.nvidia.com/accelerator`
- `network.topology.nvidia.com/tier-0`
- `network.topology.nvidia.com/tier-1`
- `network.topology.nvidia.com/tier-2`

NFD `NodeFeatureGroup` is an alpha NFD API. NFD master watches
`NodeFeatureGroup` objects, evaluates their feature-group rules, and writes the
matching nodes into `.status.nodes`. The rules use the same feature-matching
model as NFD `NodeFeatureRule`. See the NFD
[custom resources](https://kubernetes-sigs.github.io/node-feature-discovery/master/usage/custom-resources.html#nodefeaturegroup)
and
[API reference](https://kubernetes-sigs.github.io/node-feature-discovery/master/reference/generated-nfd-api-reference.html#nodefeaturegroup)
for the current resource shape.

## Goals

- Add a new engine named `nfd`.
- Keep provider output unchanged. Providers still return `topology.Graph`.
- Reuse the k8s engine's graph-to-topology-label mapping.
- Create `NodeFeatureGroup` objects for distinct topology values.

## Non-Goals

- Do not make NFD required for existing Kubernetes deployments.

## Proposed Behavior

The `nfd` engine should perform three steps:

1. Build the same per-node topology map produced by the `k8s` engine, but do not create node labels.
2. Publish per-node topology values as NFD `NodeFeature` input.
3. Create or update one `NodeFeatureGroup` per distinct topology value.

Example generated feature input for `node-a`:

```yaml
apiVersion: nfd.k8s-sigs.io/v1alpha1
kind: NodeFeature
metadata:
  name: topograph-node-a
  labels:
    nfd.node.kubernetes.io/node-name: node-a
    app.kubernetes.io/managed-by: topograph
spec:
  features:
    attributes:
      system.name:
        elements:
          nodename: node-a
      topograph.network:
        elements:
          accelerator: nvl3
          fabric-tier-0: leaf-12
          fabric-tier-1: spine-2
          fabric-tier-2: core-1
```

Example generated group for one leaf switch:

```yaml
apiVersion: nfd.k8s-sigs.io/v1alpha1
kind: NodeFeatureGroup
metadata:
  name: topograph-fabric-tier-0-leaf-12-...
  labels:
    app.kubernetes.io/managed-by: topograph
    topograph.nvidia.com/group-type: fabric-tier-0
  annotations:
    topograph.nvidia.com/label-key: network.topology.nvidia.com/tier-0
    topograph.nvidia.com/label-value: leaf-12
spec:
  featureGroupRules:
    - name: fabric-tier-0 equals leaf-12
      matchFeatures:
        - feature: topograph.network
          matchExpressions:
            fabric-tier-0:
              op: In
              value: ["leaf-12"]
```

NFD owns `.status.nodes`; Topograph owns only the desired CR specs and metadata.
Group names should be stable and Kubernetes-safe, using a short hash when the
raw topology value is too long or contains invalid name characters.

## Engine Parameters

Initial parameters:

- `nodeSelector`: optional, same meaning as the `k8s` engine.
- `cleanup`: optional boolean, default `true`, deleting stale Topograph-managed
  `NodeFeature` and `NodeFeatureGroup` objects no longer present in the latest
  graph. An empty generated object set is rejected while cleanup is enabled so
  a transient empty provider result cannot delete the entire published
  topology.

The NFD master namespace is deployment-scoped rather than request-scoped. Helm
configures it through the top-level `nfdNamespace` value and passes it to the
engine as `NFD_NAMESPACE`; non-Helm deployments set that environment variable
directly. The Helm value defaults to `node-feature-discovery`; the engine
returns an error if `NFD_NAMESPACE` is unset or blank.

## Implementation Notes

- Add `pkg/engines/nfd` with the standard `NamedLoader`.
- Register it in `pkg/registry/registry.go`.
- Factor the current `k8s` label projection into a shared helper so both engines
  produce identical values at every discovered fabric tier and accelerator domain.
- Use the dynamic Kubernetes client or generated NFD client types, depending on
  whether the project wants to pin an NFD API dependency.
- Update Helm RBAC to allow create, update, patch, list, watch, and delete for
  `nodefeatures` and `nodefeaturegroups` in `nfd.k8s-sigs.io`.
- Add `nfd` to chart value validation and engine documentation only if the
  implementation ships, not while this remains a proposal.

## Scheduling Caveat

NFD might not be the right place for this feature.

If Topograph creates a `NodeFeatureGroup` for each leaf switch, then a consumer
must know which group represents which leaf switch. A scheduler that wants to
place a workload within a single leaf switch must distinguish between all
available leaf-switch groups before it can choose one.

This is different from finding all nodes with GPUs. In the GPU case, both the
label name and label value are identical across all matching nodes, for example
`feature.node.kubernetes.io/pci-10de.present=true`. In the leaf-switch case, the
label name is the same on every node, but the label value differs from one switch
to another.

The current Kubernetes label model handles this naturally: pod affinity can use
`topologyKey: network.topology.nvidia.com/tier-0`, and Kubernetes compares values
on candidate nodes. `NodeFeatureGroup` exposes precomputed groups instead, so
the consumer needs extra logic to choose among them.

Two possible ways to make the NFD model useful for scheduling are:

- Extend pod affinity or a related scheduling API so a `topologyKey` can be
  derived from `NodeFeatureGroup` membership.
- Build a scheduler plugin that watches `NodeFeatureGroup` objects and
  understands Topograph's group metadata.

Without one of those, the `nfd` engine is mainly an integration artifact for NFD
consumers, not a complete scheduling solution.

## Future NFD Enhancements

Two NFD-side extensions could make this model more practical:

- `NodeFeatureGroup` union rules. For higher switch tiers, Topograph already has
  groups for the underlying tier: a spine group is the union of its leaf-switch
  groups, and a core group is the union of its spine-switch groups. NFD could
  support declaring a `NodeFeatureGroup` as the union of existing
  `NodeFeatureGroup` objects instead of restating the underlying feature
  matches.
- Compressed node membership in status. On large clusters, especially clusters
  with more than 1000 nodes, listing every matched node in every group can make
  `NodeFeatureGroup` status large and increase etcd storage pressure. NFD could
  consider compact node-name ranges such as `node[100-199]` when node names are
  compatible with range compression.

## etcd Size Estimate

For a 10,000-node cluster, the rough live custom-resource payload is likely in
the 10-30 MiB range before etcd MVCC history, compaction effects, and filesystem
overhead.

Assumptions:

- 10,000 `NodeFeature` objects, one per node.
- One topology attribute for every discovered fabric tier and accelerator domain.
- Each node appears in one `NodeFeatureGroup.status.nodes` list per topology
  dimension, so status contains about 40,000 node references total.
- Average node names and topology values are short, roughly 10-30 characters.
- Updates avoid large `managedFields` payloads and unnecessary annotations.

Estimated live payload:

| Object data | Count | Per-item estimate | Total |
|---|---:|---:|---:|
| `NodeFeature` objects | 10,000 | 0.7-1.5 KiB | 7-15 MiB |
| `NodeFeatureGroup` specs and metadata | ~1,500-2,000 groups | 0.8-1.5 KiB | 1-3 MiB |
| `NodeFeatureGroup.status.nodes` entries | 40,000 memberships | 25-50 B | 1-2 MiB |

The group count depends on topology fanout, but the status membership count is
mostly stable: each node appears once per published topology dimension. A single
large group, such as one core switch containing all 10,000 nodes, would carry a
large status list of roughly 250-500 KiB with short node names. That is probably
workable, but it gets closer to Kubernetes object-size limits if node names are
long or status later grows more fields.

The practical storage budget should be higher than the live payload estimate.
During topology refreshes, etcd keeps old revisions until compaction and
database defragmentation. If Topograph updates every `NodeFeature` and
`NodeFeatureGroup` on every run, temporary and on-disk usage can grow well above
the live 10-30 MiB payload. The implementation should skip no-op updates and use
small patches to reduce write amplification.

## Test Plan

- Unit-test graph-to-group generation across variable fabric tiers and accelerator domains.
- Verify long and invalid topology values produce stable CR names.
- Verify stale Topograph-managed groups are removed when `cleanup` is enabled.
- Verify an empty generated object set returns an error and preserves existing
  objects when `cleanup` is enabled.
- Verify nodes with `nvidia.com/gpu.clique` follow the same accelerator behavior
  as the `k8s` engine.
- Add a fake dynamic-client test that applies generated `NodeFeature` and
  `NodeFeatureGroup` objects without requiring a live NFD deployment.
