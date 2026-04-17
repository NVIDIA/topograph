# InfiniBand Topology Provider

Topograph provides two variations of InfiniBand provider. Both discover the IB fabric switch tree using `ibnetdiscover`, which is useful for any cluster — CPU-only, mixed, or GPU-accelerated — where topology-aware scheduling across an InfiniBand fabric improves workload performance. NVLink domain discovery is an additional capability that applies only to nodes with NVLink-connected NVIDIA GPUs.

**Why automate IB discovery?** Hand-maintaining IB topology — a static `topology.conf` or a set of hand-applied node labels — is feasible at ~32 nodes with a stable network and a careful operator. It does not scale. At 1,000 nodes with InfiniBand fabric churn, NVLink partitions shifting with tenant allocation, and a constant background rate of link degradation and node cycling, manual maintenance becomes the dominant source of scheduling misplacement. Topograph keeps topology data current as the cluster changes, removing that burden.

The choice of which to use depends on the specifics of the deployment environment:

- Use **`infiniband-bm`** for bare-metal clusters (e.g. Slurm)
- Use **`infiniband-k8s`** for Kubernetes clusters

If **NetQ is deployed** in your environment, consider using the [NetQ provider](./netq.md) instead — it discovers topology via the NetQ management API rather than directly from the fabric, which avoids node access requirements and is the standard approach for Spectrum-X environments.

For **Multi-Node NVLink (MNNVL) Kubernetes clusters** (e.g. GB200 NVL72), use the [DRA provider](./dra.md) instead — it reads `nvidia.com/gpu.clique` labels set by the GPU Operator's DRA driver and is the Kubernetes-native integration path for MNNVL topology.

| | `infiniband-bm` | `infiniband-k8s` |
|---|---|---|
| **Auth** | None | In-cluster service account |
| **Node access** | `pdsh` (SSH-based) | Kubernetes pod exec |
| **NVLink clique source** | `nvidia-smi` via pdsh | Node annotations (set by node-data-broker) |
| **Target environment** | Bare-metal / Slurm | Kubernetes |

Both variants are presently single-region only (multi-region requests return a `400 Bad Request` error). No CSP credentials are required.

## Output

Both variants produce the same topology representation, and are in turn consumed by whichever engine you configure:

- **Slurm engine** (`engine: slurm`) — writes a `topology.conf` file describing the switch tree, used by the Slurm topology plugin for topology-aware scheduling
- **Kubernetes engine** (`engine: k8s`) — applies `network.topology.nvidia.com/` labels to nodes reflecting their position in the switch hierarchy and (where applicable) their NVLink domain
- **Slinky engine** (`engine: slinky`) — writes topology data to a Kubernetes ConfigMap for Slurm-on-Kubernetes deployments

See the engine documentation (`docs/engines/`) for details on each output format.

---

## `infiniband-bm` (Bare-Metal)

### Prerequisites

- `pdsh` must be installed on the node running Topograph and able to reach at least one node per IB fabric segment — Topograph discovers the full fabric from a single entry point per segment, so every node does not need to be reachable via pdsh
- `ibnetdiscover` must be available on cluster nodes (invoked via `pdsh` with `sudo`) — part of the standard `infiniband-diags` package (`dnf install infiniband-diags` / `apt install infiniband-diags`), expected to already be present on any properly configured IB system
- NVIDIA GPU driver required on nodes with NVLink-connected GPUs — used to collect NVLink clique IDs via `nvidia-smi`. Nodes without NVLink are included in the IB switch tree but excluded from block topology.

### How It Works

1. Runs `sudo ibnetdiscover` via `pdsh` on one node per IB fabric segment to map the full switch tree
2. On NVIDIA GPU nodes: runs `nvidia-smi -q | grep "ClusterUUID\|CliqueId" | sort -u` via `pdsh` across all nodes to collect NVLink clique IDs
3. Combines the switch tree and any NVLink clique data into the topology graph

### Configuration

No credentials or parameters are required. Set `provider: infiniband-bm` in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: infiniband-bm
engine: slurm
```

### Verifying the Output

After triggering topology generation, query the result endpoint:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)
curl -s "http://localhost:49021/v1/topology?uid=$id"
```

For the Slurm engine, verify the generated `topology.conf` reflects the expected switch hierarchy. See the [Slurm engine documentation](../engines/slurm.md) for details.

---

## `infiniband-k8s` (Kubernetes)

### Prerequisites

- Topograph deployed via Helm — the node-data-broker DaemonSet (a Topograph subchart, enabled by default) collects NVLink clique IDs from each node and stores them as Kubernetes node annotations (`topograph.nvidia.com/cluster-id`)
- NVIDIA GPU Operator — standard on NVIDIA GPU Kubernetes clusters; manages the device plugin DaemonSet used to read NVLink clique IDs. Required only for NVLink domain discovery; on clusters without NVLink-connected GPUs this does not apply and the provider will still discover the IB switch tree.

### How It Works

1. Runs `ibnetdiscover` by exec-ing into a node-data-broker pod on each node to map the switch tree
2. On NVIDIA GPU nodes: reads NVLink clique IDs from the `topograph.nvidia.com/cluster-id` node annotations set by the node-data-broker
3. Combines the switch tree and any NVLink clique data into the topology graph

### Configuration

No credentials are required. The provider uses the in-cluster service account automatically.

Set `provider: infiniband-k8s` in your Topograph config:

```yaml
http:
  port: 49021
  ssl: false

provider: infiniband-k8s
engine: k8s
```

### Parameters

The following optional parameter can be passed in the topology request payload:

| Parameter | Type | Default | Description |
|---|---|---|---|
| `nodeSelector` | `map[string]string` | — | Label selector to filter which nodes participate in topology discovery |

To override the GPU Operator namespace or device plugin DaemonSet name (defaults: `gpu-operator` and `nvidia-device-plugin-daemonset`), set these via `node-data-broker.initc.extraArgs` in your Helm values — they are init container arguments, not provider request parameters:

```yaml
node-data-broker:
  initc:
    extraArgs:
      - gpu-operator-namespace=my-namespace
      - device-plugin-daemonset=my-daemonset
```

Example request payload with `nodeSelector`:

```json
{
  "provider": {
    "name": "infiniband-k8s",
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

### Verifying the Output

After topology generation, inspect the node labels applied by Topograph:

```bash
kubectl get nodes -o json | jq '.items[].metadata.labels | with_entries(select(.key | startswith("network.topology.nvidia.com")))'
```

See the [Kubernetes engine documentation](../engines/k8s.md) for details on the label schema.
