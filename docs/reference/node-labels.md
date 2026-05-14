# Topograph Node Labels and Annotations

Topograph enriches Kubernetes nodes with labels and annotations that describe their physical network topology. This reference covers every label and annotation key written by Topograph, how values are derived, and how to configure them.

## Labels

Labels are set by the [Kubernetes engine](../engines/k8s.md) (`engine: k8s`) and the [Slinky engine](../engines/slinky.md) (`engine: slinky`). They are intended for use by workload schedulers (e.g. KAI Scheduler, gang-scheduling plugins, topology-aware bin-packers) and observability tools to reason about network locality.

### Default label keys

| Label key | Topology type | Semantics |
|---|---|---|
| `network.topology.nvidia.com/accelerator` | Block (`topology/block`) | Accelerated interconnect domain identifier — nodes that share the same value are in the same accelerated domain. Exact semantics are provider-dependent: for MNNVL-aware providers (DRA, InfiniBand, Lambda AI) the value is an NVL Partition identifier (Fabric-Manager-derived `<ClusterUUID>.<CliqueID>`, identifying a logical sub-domain within the physical NVL Domain); for the AWS provider it is the AWS CapacityBlockId (a reservation-scoped identifier co-extensive with an UltraServer — i.e., the NVL Domain — on P6e-GB200). See the provider matrix below. |
| `network.topology.nvidia.com/leaf` | Tree (`topology/tree`) | Leaf switch identifier — top-of-rack or first-tier fabric switch |
| `network.topology.nvidia.com/spine` | Tree (`topology/tree`) | Spine switch identifier — second-tier aggregation switch |
| `network.topology.nvidia.com/core` | Tree (`topology/tree`) | Core switch identifier — third tier, present in large three-tier fabrics |

Labels are **additive**: a node that belongs to both a block topology (NVLink domain) and a tree topology (switch fabric) carries both `accelerator` and `leaf`/`spine`/`core` simultaneously.

Not all providers produce both topology types:

| Provider | Block (`accelerator`) | Tree (`leaf`/`spine`/`core`) |
|---|---|---|
| `aws` | Yes (CapacityBlockId) | Yes |
| `cw` | No | Yes (InfiniBand switch hierarchy) |
| `gcp` | No | Yes |
| `lambdai` | Yes (`NVLink.DomainID.CliqueID`) | Yes |
| `oci` | No | Yes |
| `nebius` | No | Yes |
| `nscale` | Yes | Yes |
| `netq` | Yes (NMX `DomainUUID`) | Yes (Spectrum-X switch hierarchy) |
| `dra` | Yes (reads `nvidia.com/gpu.clique`) | No |
| `infiniband-bm` | Yes (`ClusterUUID.CliqueId`) | Yes (IB switch hierarchy) |
| `infiniband-k8s` | Yes (`ClusterUUID.CliqueId`) | Yes (IB switch hierarchy) |

**Relationship to `nvidia.com/gpu.clique`**: The GPU Operator device plugin sets `nvidia.com/gpu.clique` on nodes with Multi-Node NVLink (MNNVL) GPUs. The `infiniband-bm` and `infiniband-k8s` providers derive their `accelerator` value from the same `ClusterUUID.CliqueId` hardware identifiers, so the values are directly comparable. The `netq` provider uses a `DomainUUID` from the NMX management API — a different identifier that refers to the same physical domain but cannot be compared as a string.

[NVIDIA Fabric Manager](https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/) runs at node init on MNNVL-capable hardware, discovers the NVLink fabric across GPUs, and registers each GPU with [NVML](https://docs.nvidia.com/deploy/nvml-api/) (NVIDIA Management Library — a C API that exposes per-GPU state). The GPU Operator's IMEX labeler writes `nvidia.com/gpu.clique` only once NVML reports the node's fabric state as `GPU_FABRIC_STATE_COMPLETED` — meaning Fabric Manager finished initialization successfully and the node is part of an NVLink domain.

On non-MNNVL systems (e.g., DGX B200, B300), the GPU fabric never reaches `GPU_FABRIC_STATE_COMPLETED`, so `nvidia.com/gpu.clique` is not set at all. On these systems, Topograph with an InfiniBand provider is the only source of network topology for scheduling decisions.

### Choosing between `accelerator` and `nvidia.com/gpu.clique` for scheduling

Workload schedulers consuming topology labels may need to choose between Topograph's `network.topology.nvidia.com/accelerator` and the NVIDIA GPU Operator's `nvidia.com/gpu.clique`. The right choice depends on the provider and the desired granularity:

- **MNNVL hardware + Fabric Manager completed + NVL Partition granularity desired:** prefer `nvidia.com/gpu.clique`. On the AWS provider this is finer granularity than `accelerator` (which carries the CapacityBlockId, i.e., the NVL Domain). On DRA, InfiniBand, and Lambda AI providers the two labels carry the same value.
- **MNNVL but Fabric Manager not yet completed, or non-MNNVL hardware:** `nvidia.com/gpu.clique` is absent. Use `network.topology.nvidia.com/accelerator`.
- **Slurm clusters (no Kubernetes node labels):** neither label applies. Consumers read Slurm's `topology.conf` directly.

**Caveats when preferring `nvidia.com/gpu.clique`:**

- The label encodes node identity within MNNVL domains, not fabric proximity between them. NVL Partition is encoded as the full `<ClusterUUID>.<CliqueID>` value; NVL Domain is encoded as the `ClusterUUID` prefix. A scheduler can therefore distinguish racks — two nodes with different `ClusterUUID` are in different NVL Domains — and act on that distinction (same-Domain affinity to pack a job onto a single rack, cross-Domain anti-affinity to spread independent jobs across racks). What the label does **not** encode is the *physical proximity* between Domains: `ClusterUUID`s are opaque identifiers, so the label cannot tell a scheduler which racks share a top-of-rack switch, an aggregation tier, or a core. For cross-rack proximity-aware placement, Topograph populates the following labels from the InfiniBand or NetQ providers regardless of whether `gpu.clique` is present:
    - **Same top-of-rack switch** (cross-rack within a first-tier fabric) — Topograph's `leaf` label.
    - **Same second-tier aggregation** (typically Scalable-Unit / pod-scale grouping above individual racks) — Topograph's `spine` label.
    - **Same third-tier aggregation** (present in large three-tier fabrics — typically cross-SU grouping in multi-SU SuperPOD deployments) — Topograph's `core` label.

  These labels are also relevant for mixed-workload fragmentation avoidance (see [`docs/engines/k8s.md` § Mixed Workload Considerations](../engines/k8s.md#mixed-workload-considerations)).
- The label is refreshed by GPU Feature Discovery at its configured interval (the k8s-device-plugin default is 60s) rather than propagated instantly. Fabric-state changes in the window between refreshes are not yet reflected in the label.
- Persistence of `ClusterUUID` / `CliqueID` across node reboots is administratively controlled via Fabric Manager's `FABRIC_MODE_RESTART` configuration (default: preserve partition configurations). Deployments that disable preservation may see identifiers change across restarts, which can invalidate scheduler state cached on those values.

### Label value behavior

Label values are used as-is when they are 63 characters or shorter (the Kubernetes label value limit). Values longer than 63 characters are replaced with their **FNV-64a hash** rendered as an `x`-prefixed lowercase hex string (e.g., `x3e4f1a2b3c4d5e6f`) to stay within the limit. This means two nodes with the same long switch identifier will carry the same hash value — locality is preserved, but the original identifier is not recoverable from the label alone.

### Configuring label keys

The default `network.topology.nvidia.com/` prefix is configurable via the Helm `topologyNodeLabels` value. If you need to map topograph's topology layers to a custom label schema, override the keys at deploy time. The label _values_ (topology identifiers) are always derived from the provider's topology discovery and cannot be configured.

### Relationship to upstream standardization (KEP-4962)

An active Kubernetes Enhancement Proposal (KEP), [KEP-4962: Standardizing the Representation of Cluster Network Topology](https://github.com/kubernetes/enhancements/issues/4962) ([draft in PR #4965](https://github.com/kubernetes/enhancements/pull/4965)), advocates reserved label keys under the `topology.kubernetes.io/` namespace for a standardized representation of cluster network topology. The KEP is pre-GA and still under upstream review. Topograph's current `network.topology.nvidia.com/*` keys predate any potential upstream standard and are presently vendor-scoped — the KEP's framing allows vendor prefixes and standard labels to coexist rather than replace one another. If KEP-4962 reaches GA with stable keys, Topograph will evaluate aligning or providing both; for now, the `network.topology.nvidia.com/*` keys remain authoritative for Topograph-deployed clusters.

## Without Topograph

When Topograph is not deployed, the labels commonly available for topology-aware scheduling are:

| Label key | Source | Semantics |
|---|---|---|
| `topology.kubernetes.io/zone` | Cloud provider / kubelet | Availability zone or data center zone |
| `topology.kubernetes.io/region` | Cloud provider / kubelet | Geographic region |
| `node.kubernetes.io/instance-type` | Cloud provider | VM / instance SKU |
| `topology.k8s.aws/capacity-block-id` | AWS Node Feature Discovery | AWS Capacity Block reservation ID. Per the [EC2 API reference for `InstanceTopology`](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_InstanceTopology.html), on UltraServer instances this "identifies instances within the UltraServer domain" — a reservation-scoped grouping, not an NVL Partition identifier. On P6e-GB200 it is co-extensive with one UltraServer (AWS requires reserving the UltraServer as a unit per the [EKS UltraServer guide](https://docs.aws.amazon.com/eks/latest/userguide/ml-eks-nvidia-ultraserver.html)), so it aligns with the NVL Domain. AWS surfaces an explicit NVL Domain label, [`topology.k8s.aws/ultraserver-id`](https://docs.aws.amazon.com/sagemaker/latest/dg/sagemaker-hyperpod-eks-operate-console-ui-governance-tasks-scheduling.html), on SageMaker HyperPod-managed EKS clusters; on plain EKS or self-managed Kubernetes on P6e-GB200, AWS does not apply that label, and the NVL Domain must be derived from `nvidia.com/gpu.clique` (its `<ClusterUUID>.<CliqueID>` value encodes the NVL Domain as the ClusterUUID prefix). Topograph's AWS provider derives `network.topology.nvidia.com/accelerator` from the same `CapacityBlockId` attribute, so on AWS the two labels carry identical string values — Domain-scoped, not Partition-scoped. |
| `topology.k8s.aws/network-node-layer-1` | AWS Node Feature Discovery | AWS network spine |
| `topology.k8s.aws/network-node-layer-2` | AWS Node Feature Discovery | AWS network aggregation |
| `topology.k8s.aws/network-node-layer-3` | AWS Node Feature Discovery | AWS network leaf |
| `oci.oraclecloud.com/host.network_block_id` | OCI | OCI network block |
| `oci.oraclecloud.com/host.rack_id` | OCI | OCI rack |
| `cloud.google.com/gce-topology-block` | GCP | GCP topology block |
| `cloud.google.com/gce-topology-subblock` | GCP | GCP topology sub-block |
| `cloud.google.com/gce-topology-host` | GCP | GCP host |
| `nvidia.com/gpu.clique` | NVIDIA GPU Operator (device plugin) | NVL Partition identifier, formatted `<ClusterUUID>.<CliqueID>`. The `ClusterUUID` prefix identifies the physical NVL Domain (e.g., one GB200 NVL72 rack); the `CliqueID` suffix identifies a Fabric-Manager-assigned logical sub-domain within it. Set only on MNNVL-capable nodes once Fabric Manager completes initialization and NVML reports `NVML_GPU_FABRIC_STATE_COMPLETED`; not present on non-MNNVL systems and may be absent on MNNVL nodes where Fabric Manager init has not completed. Multiple clique values can appear within a single NVL Domain (e.g., an x72 UltraServer split into two x36 halves). |
| `nvidia.com/cuda.driver-version.full` | NVIDIA GPU Operator (GFD) | Full CUDA driver version |
| `nvidia.com/cuda.runtime-version.full` | NVIDIA GPU Operator (GFD) | Full CUDA runtime version |

These labels are set by cloud provider integrations and the NVIDIA GPU Operator's GPU Feature Discovery (GFD) component — not by Topograph.

## Annotations

Topograph sets the following annotations on nodes as internal bookkeeping metadata. These are not intended for scheduler use but may be useful for debugging and observability.

| Annotation key | Semantics |
|---|---|
| `topograph.nvidia.com/instance` | The cloud instance ID or node identifier as returned by the provider |
| `topograph.nvidia.com/region` | The provider region associated with this node |
| `topograph.nvidia.com/cluster-id` | The cluster identifier (where reported by the provider) |

Additional annotations are set on topology ConfigMaps (used by the Slinky engine):

| Annotation key | Semantics |
|---|---|
| `topograph.nvidia.com/engine` | The engine that generated the ConfigMap |
| `topograph.nvidia.com/topology-managed-by` | The Topograph instance managing the ConfigMap |
| `topograph.nvidia.com/last-updated` | Timestamp of the most recent topology update |
| `topograph.nvidia.com/plugin` | The scheduler plugin that consumes the ConfigMap |
| `topograph.nvidia.com/block-sizes` | Comma-separated list of block sizes in the topology |
| `topograph.nvidia.com/slurm-namespace` | The Slurm namespace associated with this topology ConfigMap |

## Integration with NVSentinel

NVSentinel's Metadata Augmentor enriches health events with node labels from a configurable `allowedLabels` list. As of [NVSentinel #1226](https://github.com/NVIDIA/NVSentinel/pull/1226) (merged 2026-04-23; shipping in the next NVSentinel release), the four `network.topology.nvidia.com/*` labels are included in the default `allowedLabels` — so on clusters where Topograph is deployed, NVSentinel propagates topology into health event metadata automatically, with no operator configuration required. Downstream consumers — fault-quarantine CEL rules, remediation custom resources, dashboards, blast-radius analysis — can then reason about topological locality at NVL Partition, NVL Domain, or switch-hierarchy level.

NVSentinel's Metadata Augmentor skips labels that aren't present on a node, so nodes without Topograph (or MNNVL-only labels on non-MNNVL hardware) behave cleanly — no configuration conditionals needed.

Operators on earlier NVSentinel versions, or operators running a customized `allowedLabels` list, can add the Topograph labels explicitly in `distros/kubernetes/nvsentinel/values.yaml`:

```yaml
transformers:
  MetadataAugmentor:
    allowedLabels:
      # ... existing labels ...
      # Topograph topology labels (requires Topograph deployed in the cluster)
      - "network.topology.nvidia.com/accelerator"
      - "network.topology.nvidia.com/leaf"
      - "network.topology.nvidia.com/spine"
      - "network.topology.nvidia.com/core"
```

See NVSentinel's [`docs/INTEGRATIONS.md` § Topology Awareness (Topograph)](https://github.com/NVIDIA/NVSentinel/blob/main/docs/INTEGRATIONS.md#topology-awareness-topograph).
