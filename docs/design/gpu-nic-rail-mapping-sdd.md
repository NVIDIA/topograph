# Software Design Document: GPU-to-NIC Rail Mapping

## Status

Draft.

## Decision Summary

GPU-to-NIC rail mapping is an optional Kubernetes node-metadata plugin,
independent of `provider.name` and `topology.Graph`.

- The node-data-broker discovers and publishes the record for its Node.
- The Node Observer creates and owns the shared ConfigMap and removes stale
  records.
- A rail-source interface supports host LLDP initially and a future DOCA
  XPlane adapter when a supported external topology contract is available.
- Helm is one deployment mechanism for wiring the feature; its values, mounts,
  and RBAC are not part of the feature's internal interfaces.

## Goals and Non-Goals

The feature publishes, for each physical GPU, the closest host-visible NICs on
every rail. GPU UUIDs and normalized PCI addresses are stable identities; the
NVIDIA path class is retained for explainability. The same plugin can be
composed with any Kubernetes provider that runs the broker and observer.

The feature does not modify the canonical topology graph, discover the full
switch fabric, allocate devices, advertise kubelet resources, validate
GPUDirect RDMA, measure network health, or publish MIG-device mappings. It is a
compiled-in Topograph extension, not an external executable or Go plugin ABI.

## Feature Implementation

### Responsibilities and Data Flow

| Component | Responsibility |
|---|---|
| Node-data-broker plugin | Discover local NIC rails and GPU locality; publish its Node record |
| Node Observer | Create and own the shared ConfigMap; remove deleted or replaced Node records |
| Topology provider | Continue producing canonical fabric and accelerator topology independently |

```text
rail source  -> NIC PCI address -> rails --+
                                             +-> join and select -> Node record
GPU source   -> GPU/NIC PCI locality -------+

Node-data-broker -> upsert its Node record
Node Observer    -> own ConfigMap and garbage-collect stale records
```

The broker loads compiled-in node-metadata collectors from a small registry.
The Node Observer loads the corresponding object reconciler when the same
plugin is enabled. Provider annotation collection remains a separate broker
step.

The current LLDP broker path returns provider annotations and NIC rails in one
result. Implementation must split that path: the `lldp-k8s` provider collector
returns only its annotations, while the plugin invokes reusable LLDP parsing
and device-resolution helpers independently.

### Rail Sources

A rail source implements this logical operation:

```text
DiscoverNICRails(context) -> map[host PCI address][]rail ID
```

Output PCI addresses use lowercase `dddd:bb:ss.f` form and rail arrays are
sorted and deduplicated. One host PCI function may belong to multiple rails.
There is no automatic fallback between sources.

#### Host LLDP

The initial source runs `lldpctl -f json` against the host `lldpd` socket,
selects interfaces using a configured regular expression, and expands each
match through a rail-ID template. It resolves each selected interface through:

```text
/sys/class/net/<interface>/device
```

Different interfaces may report different chassis IDs; this is normal for a
multi-rail node. Any ambiguity rules used by the `lldp-k8s` topology provider
are separate from rail discovery.

This source is feasible only when the selected host interfaces identify the
physical rails and resolve to the PCI functions visible to the GPU topology
view. Bond, bridge, VF, representor, and aggregated interfaces are not
implicitly unwrapped.

#### XPlane (Future)

Public NVIDIA documentation establishes the
[DOCA xPlane sidecar](https://docs.nvidia.com/networking/display/kubernetes2640/spectrum-x/components.html)
and the BF4 ASTRA
[`doca_xplane` service](https://docs.nvidia.com/infra-controller/documentation/getting-started/installation-options/dpf-setup),
but not a supported external topology API suitable for this feature.

An XPlane adapter is therefore design-only until a supported contract defines:

- a stable rail or group identifier;
- the corresponding host-visible NIC PCI address;
- API version, transport, authentication, and service location; and
- enough plane or port identity to reject contradictory records.

The adapter must not guess host identity from DPU PCI addresses, representor
names, interface order, or other platform enumeration.

### GPU and NIC Locality

On a GPU Node, the broker executes these commands in exactly one configured,
same-node GPU Operator pod:

```text
nvidia-smi --query-gpu=index,uuid,pci.bus_id --format=csv,noheader
nvidia-smi topo -m
```

The first command maps temporary matrix indices to stable GPU UUID and PCI
identities. The matrix supplies GPU-to-NIC path classes. NIC aliases are
resolved through `/sys/class/infiniband/<device>/device`, producing the same
host PCI identity used by the rail source.

The broker selects the GPU Operator pod using the configured DaemonSet's full
label selector and verifies that the pod is controlled by that DaemonSet. The
design uses `topo -m` because the focused `topo -nic` command requires R610,
while supported GPU Operator deployments include earlier drivers.

### Affinity Selection

Path classes rank closest-first:

| Rank | Class | Meaning |
|---:|---|---|
| 1 | `PIX` | At most one PCIe bridge |
| 2 | `PXB` | Multiple PCIe bridges, no host bridge |
| 3 | `PHB` | Crosses a PCIe host bridge |
| 4 | `NODE` | Crosses host bridges within one NUMA node |
| 5 | `SYS` | Crosses NUMA nodes |

For each GPU and rail, selection retains every NIC tied at the best rank and
sorts ties by PCI address. Selection is independent per rail. Unknown path
classes and a rail with no qualified NIC candidate fail the Node update. NICs
present in `nvidia-smi` but absent from rail discovery are ignored.

### Shared Record Contract

The shared ConfigMap uses the Kubernetes Node name as each `data` key. The
value is one versioned JSON record replaced atomically by that Node's broker:

```json
{
  "schemaVersion": "v1alpha1",
  "nodeUID": "3e931c2b-6a5d-4c17-93bc-bc3a34132bd2",
  "railSource": "host-lldp",
  "nics": {
    "0000:3b:00.0": ["rail0"],
    "0000:af:00.0": ["rail1"]
  },
  "gpus": {
    "GPU-acde1234-acde-1234-acde-1234abcdef01": {
      "pciAddress": "0000:17:00.0",
      "rails": {
        "rail0": [{"nic": "0000:3b:00.0", "path": "PIX"}],
        "rail1": [{"nic": "0000:af:00.0", "path": "SYS"}]
      }
    }
  }
}
```

Contract rules:

- Consumers reject unknown `schemaVersion` values.
- `nodeUID` protects against Node-name reuse and supports safe cleanup.
- `nics` maps host PCI functions to sorted rail arrays.
- `gpus` is keyed by physical GPU UUID and is empty on a non-GPU Node.
- `path` is the selected NVIDIA topology class.

### Publication and Object Ownership

The Node Observer ensures the ConfigMap exists before reconciling Node records.
It creates the object with a controller owner reference to its Deployment and
refuses to adopt an unrelated same-named ConfigMap. Concurrent observer
creation is idempotent: `AlreadyExists` causes ownership validation.

Each broker patches only its key, without a read-modify-write of the complete
ConfigMap:

```json
{"data":{"node-a":"<complete versioned record>"}}
```

Concurrent brokers therefore update independent keys. A successful empty rail
result removes that broker's key. Discovery or publication errors preserve the
previous value and keep the broker unhealthy. A broker that starts before the
ConfigMap exists retries with backoff.

The Node Observer removes a record when its Node no longer exists or its live
UID differs from the record's `nodeUID`. It reads the value and uses one atomic
JSON Patch to test the complete value and remove the key. If a replacement Node
has already published a new record, the test fails and the observer
re-evaluates current state. A full reconciliation after informer cache sync
cleans events missed during observer downtime.

The observer deletes an owned ConfigMap when the plugin is disabled. The owner
reference lets Kubernetes garbage collection remove it with the observer
Deployment.

### Failure Semantics and Limitations

| Condition | Result |
|---|---|
| Non-GPU Node | Publish NIC rails with an empty `gpus` object |
| No or multiple eligible GPU Operator pods on a GPU Node | Fail and preserve the previous record |
| Empty successful rail discovery | Remove this Node's record |
| Rail, GPU, parsing, join, or publication error | Fail and preserve the previous record |
| Malformed record for a live Node | Observer reports it; broker is responsible for replacement |

The broker currently reconciles once at startup, so in-place hardware changes
require a restart. The ConfigMap has an approximate 1 MiB limit; large clusters
may require one object per Node or a custom resource. `nvidia-smi topo -m`
describes physical GPUs; MIG devices inherit the parent mapping but are not
published separately. The result describes locality, not operational RDMA.

### Implementation Validation

Unit and integration tests cover:

- rail-source parsing, PCI normalization, multi-chassis LLDP, and multi-rail
  PCI functions;
- GPU inventory, topology parsing, joins, path ranking, ties, and error cases;
- deterministic records and concurrent per-key publication;
- ConfigMap creation, ownership validation, and unrelated-object rejection;
- deleted Nodes, observer restart, Node-name reuse, and conditional-delete
  races; and
- composition with representative Kubernetes providers without changing their
  graph output.

Hardware validation covers a pre-R610 driver, a multi-GPU/multi-rail Node,
equal-distance ties, MIG mode, a non-GPU Node, and non-rail or bonded NICs.
XPlane validation is deferred until its external contract is confirmed.

## Deployment Mechanism

This section describes the proposed Helm integration. It may change without
changing the feature interfaces or record contract above.

### Values

The feature is configured once, outside provider and component-specific
sections:

```yaml
provider:
  name: <topology-provider>

nodeDataBroker:
  enabled: true

nodeMetadata:
  plugins:
    gpuNicRailMapping:
      enabled: true
      configMapName: topograph-nic-rails
      railSource:
        name: host-lldp
        hostLLDP:
          interfaceRegex: '^eth_r([0-9]+)_p[0-9]+$'
          railID: 'rail$1'
      gpuSource:
        gpuOperatorNamespace: gpu-operator
        daemonSet: nvidia-device-plugin-daemonset
```

Helm validates that the broker and observer are deployed, exactly one supported
rail source is selected, and its required settings are present. `xplane` is
rejected until its supported integration contract is defined.

### Workload Wiring

The chart does not render the shared ConfigMap; the Node Observer creates it at
runtime. The chart passes the plugin configuration and observer Deployment name
to the observer, and passes collection settings and ConfigMap identity to the
broker.

Host LLDP deployment mounts the host `lldpd` socket and `/sys/class/net`
read-only. GPU mapping mounts `/sys/class/infiniband` read-only and configures
the GPU Operator workload used for pod execution. A future XPlane adapter will
define its own wiring only after its API is supported.

### Permissions

The broker receives:

- `get` on the configured GPU Operator DaemonSet;
- `list` on Pods and `create` on `pods/exec`; and
- `patch` restricted to the configured ConfigMap name.

The Node Observer receives:

- `list` and `watch` on Nodes;
- `get` on its Deployment; and
- namespace-scoped `get`, `list`, `create`, `patch`, and `delete` on
  ConfigMaps.

Kubernetes authorization cannot restrict `create` by `resourceNames`. The
observer therefore enforces ownership in code: it creates only the configured
name and mutates or deletes only ConfigMaps controlled by its Deployment UID.

Chart tests cover values validation, conditional RBAC, mounts, generated
component configuration, plugin disablement, and broker/observer startup
ordering.

## Open Questions

1. Which identifiers will the consumer allocate: GPU UUIDs, MIG UUIDs, PCI
   addresses, or CDI names?
2. What maximum cluster size must the shared ConfigMap support?
3. Which supported XPlane API supplies both rail identity and host-visible PCI
   identity?
4. Does a consumer also require physical neighbor-switch and port identity?
