# DRA Topology Provider

The DRA provider discovers NVLink domain membership by reading `nvidia.com/gpu.clique` node labels set by the [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/index.html)'s Dynamic Resource Allocation (DRA) driver. It is a Kubernetes-only provider that uses in-cluster service account auth — no credentials are required.

**Important**: The DRA provider produces **block topology only** (`topology/block` — NVLink domain membership). It does not discover switch tree topology. If you need both switch tree and NVLink domain topology, use the [InfiniBand](./infiniband.md) or [NetQ](./netq.md) provider instead.

## Background: ComputeDomains and MNNVL

On GB200 NVL72 and similar Multi-Node NVLink (MNNVL) hardware, groups of nodes share a high-bandwidth NVLink fabric (1.8 TB/s chip-to-chip). Workloads that span these nodes — distributed training, disaggregated inference — benefit significantly from being placed within the same NVLink domain.

Kubernetes exposes this through **ComputeDomains**, a DRA-based abstraction that represents a set of nodes sharing an NVLink/MNNVL domain as a first-class scheduling object. The GPU Operator's DRA driver labels each node with `nvidia.com/gpu.clique` to encode its NVLink clique membership. Schedulers like [KAI Scheduler](https://github.com/NVIDIA/KAI-Scheduler) consume these labels — via Topograph — to make topology-aware placement decisions.

The DRA provider is Topograph's integration point for this ecosystem. For more background, see:
- [Enabling Multi-Node NVLink on Kubernetes for GB200 and Beyond](https://developer.nvidia.com/blog/enabling-multi-node-nvlink-on-kubernetes-for-gb200-and-beyond/)
- [Running Large-Scale GPU Workloads on Kubernetes with Slurm](https://developer.nvidia.com/blog/running-large-scale-gpu-workloads-on-kubernetes-with-slurm/)
- [Running AI Workloads on Rack-Scale Supercomputers: From Hardware to Topology-Aware Scheduling](https://developer.nvidia.com/blog/running-ai-workloads-on-rack-scale-supercomputers-from-hardware-to-topology-aware-scheduling/)

## When to Use This Provider

Use the DRA provider when:

- Your cluster runs Kubernetes with the NVIDIA GPU Operator and DRA support enabled
- You have MNNVL hardware (e.g. GB200 NVL72) and need NVLink domain topology for block-based scheduling
- You are using Slinky (Slurm-on-Kubernetes) with MNNVL nodes

If you also need switch tree topology — for example to express the full fabric hierarchy for `topology/tree` scheduling — use the [InfiniBand](./infiniband.md) or [NetQ](./netq.md) provider instead.

## How It Works

The `nvidia.com/gpu.clique` labels are applied automatically by the GPU Operator's DRA driver — these are not manually configured by users.

Topograph reads these labels from the Kubernetes API:

1. Lists all nodes (filtered by `nodeSelector` if provided)
2. For each node with a `nvidia.com/gpu.clique` label, reads the clique ID and groups nodes by domain
3. Returns the NVLink domain map as block topology

If no nodes with matching labels are found, Topograph returns a `502` error with a diagnostic message indicating which label and annotations to check.

## Prerequisites

- Kubernetes cluster with the NVIDIA GPU Operator deployed and DRA support enabled
- Nodes must have `nvidia.com/gpu.clique` labels — applied automatically by the DRA driver

## Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `nodeSelector` | `map[string]string` | No | Label selector to filter which nodes participate in topology discovery |

## Configuration

No credentials are required. The provider uses the in-cluster service account automatically.

Set `provider: dra` in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: dra
engine: k8s
```

For Slinky (Slurm-on-Kubernetes) deployments:

```yaml
http:
  port: 49021
  ssl: false

provider: dra
engine: slinky
```

To filter participating nodes via `nodeSelector`, pass parameters in the topology request payload:

```json
{
  "provider": {
    "name": "dra",
    "params": {
      "nodeSelector": {
        "nvidia.com/gpu.present": "true"
      }
    }
  },
  "engine": {
    "name": "k8s"
  }
}
```

## Verifying the Output

After triggering topology generation, inspect the node labels applied by Topograph:

```bash
kubectl get nodes -o json | jq '.items[].metadata.labels | with_entries(select(.key | startswith("network.topology.nvidia.com")))'
```

If topology generation returns a `502` error, check that the expected nodes have the `nvidia.com/gpu.clique` label and the `topograph.nvidia.com/region` / `topograph.nvidia.com/instance` annotations (the latter two are set by Topograph itself during topology discovery):

```bash
kubectl get nodes -o json | jq '.items[] | {name: .metadata.name, clique: .metadata.labels["nvidia.com/gpu.clique"], region: .metadata.annotations["topograph.nvidia.com/region"], instance: .metadata.annotations["topograph.nvidia.com/instance"]}'
```

See the [Kubernetes engine documentation](../engines/k8s.md) for details on the label schema.
