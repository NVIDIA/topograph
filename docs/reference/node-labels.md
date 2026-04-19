# Topograph Node Labels and Annotations

Topograph enriches Kubernetes nodes with labels and annotations that describe their physical network topology. This reference covers every label and annotation key written by Topograph, how values are derived, and how to configure them.

## Labels

Labels are set by the [Kubernetes engine](../engines/k8s.md) (`engine: k8s`) and the [Slinky engine](../engines/slinky.md) (`engine: slinky`). They are intended for use by workload schedulers (e.g. KAI Scheduler, gang-scheduling plugins, topology-aware bin-packers) and observability tools to reason about network locality.

### Default label keys

| Label key | Topology type | Semantics |
|---|---|---|
| `network.topology.nvidia.com/accelerator` | Block (`topology/block`) | NVLink domain (clique) ID — nodes that share the same NVLink fabric and can communicate at NVLink bandwidth |
| `network.topology.nvidia.com/leaf` | Tree (`topology/tree`) | Leaf switch identifier — top-of-rack or first-tier fabric switch |
| `network.topology.nvidia.com/spine` | Tree (`topology/tree`) | Spine switch identifier — second-tier aggregation switch |
| `network.topology.nvidia.com/core` | Tree (`topology/tree`) | Core switch identifier — third tier, present in large three-tier fabrics |

Labels are **additive**: a node that belongs to both a block topology (NVLink domain) and a tree topology (switch fabric) carries both `accelerator` and `leaf`/`spine`/`core` simultaneously.

Not all providers produce both topology types:

| Provider | Block (`accelerator`) | Tree (`leaf`/`spine`/`core`) |
|---|---|---|
| `aws` | Yes (CapacityBlockId) | Yes |
| `cw` | No | No (vertex structure is incompatible with the Kubernetes and Slinky engines — the provider returns a bare tree root that is not wrapped under `topology.TopologyTree`, so neither labeler processes its output; tracked separately) |
| `gcp` | No | Yes |
| `lambdai` | Yes (`NVLink.DomainID.CliqueID`) | Yes |
| `oci` | No | Yes |
| `nebius` | No | Yes |
| `netq` | Yes (NMX `DomainUUID`) | Yes (Spectrum-X switch hierarchy) |
| `dra` | Yes (reads `nvidia.com/gpu.clique`) | No |
| `infiniband-bm` | Yes (`ClusterUUID.CliqueId`) | Yes (IB switch hierarchy) |
| `infiniband-k8s` | Yes (`ClusterUUID.CliqueId`) | Yes (IB switch hierarchy) |

**Relationship to `nvidia.com/gpu.clique`**: The GPU Operator device plugin sets `nvidia.com/gpu.clique` on nodes with Multi-Node NVLink (MNNVL) GPUs. The `infiniband-bm` and `infiniband-k8s` providers derive their `accelerator` value from the same `ClusterUUID.CliqueId` hardware identifiers, so the values are directly comparable. The `netq` provider uses a `DomainUUID` from the NMX management API — a different identifier that refers to the same physical domain but cannot be compared as a string.

On non-MNNVL systems (e.g., DGX B200, B300), `nvidia.com/gpu.clique` is not set at all — the device plugin's IMEX labeler requires `GPU_FABRIC_STATE_COMPLETED`, which non-MNNVL GPUs do not reach. On these systems, Topograph with an InfiniBand provider is the only source of network topology for scheduling decisions.

### Label value behavior

Label values are used as-is when they are 63 characters or shorter (the Kubernetes label value limit). Values longer than 63 characters are replaced with their **FNV-64a hash** rendered as an `x`-prefixed lowercase hex string (e.g., `x3e4f1a2b3c4d5e6f`) to stay within the limit. This means two nodes with the same long switch identifier will carry the same hash value — locality is preserved, but the original identifier is not recoverable from the label alone.

### Configuring label keys

The default `network.topology.nvidia.com/` prefix is configurable via the Helm `topologyNodeLabels` value. If you need to map topograph's topology layers to a custom label schema, override the keys at deploy time. The label _values_ (topology identifiers) are always derived from the provider's topology discovery and cannot be configured.

## Without Topograph

When Topograph is not deployed, the labels commonly available for topology-aware scheduling are:

| Label key | Source | Semantics |
|---|---|---|
| `topology.kubernetes.io/zone` | Cloud provider / kubelet | Availability zone or data center zone |
| `topology.kubernetes.io/region` | Cloud provider / kubelet | Geographic region |
| `node.kubernetes.io/instance-type` | Cloud provider | VM / instance SKU |
| `topology.k8s.aws/capacity-block-id` | AWS Node Feature Discovery | AWS Capacity Block (NVLink domain) |
| `topology.k8s.aws/network-node-layer-1` | AWS Node Feature Discovery | AWS network spine |
| `topology.k8s.aws/network-node-layer-2` | AWS Node Feature Discovery | AWS network aggregation |
| `topology.k8s.aws/network-node-layer-3` | AWS Node Feature Discovery | AWS network leaf |
| `oci.oraclecloud.com/host.network_block_id` | OCI | OCI network block |
| `oci.oraclecloud.com/host.rack_id` | OCI | OCI rack |
| `cloud.google.com/gce-topology-block` | GCP | GCP topology block |
| `cloud.google.com/gce-topology-subblock` | GCP | GCP topology sub-block |
| `cloud.google.com/gce-topology-host` | GCP | GCP host |
| `nvidia.com/gpu.clique` | NVIDIA GPU Operator (device plugin) | NVLink clique ID — set only on MNNVL-capable nodes (e.g., GB200 NVL72); not present on non-MNNVL systems |
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

NVSentinel's Metadata Augmentor enriches GPU fault events with node labels from a configurable `allowedLabels` list (in `distros/kubernetes/nvsentinel/values.yaml`). To enable topology-aware blast-radius analysis — for example, determining whether a fault affects an entire NVLink domain or a rack — add Topograph's labels to `MetadataAugmentor.allowedLabels`:

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

These labels are only populated on nodes where Topograph has completed a topology discovery pass. On nodes without Topograph, the labels are absent and the Metadata Augmentor will skip them.
