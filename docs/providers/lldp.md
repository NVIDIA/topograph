# LLDP Topology Provider

The LLDP provider discovers each compute node's directly connected Ethernet
switch using the Link Layer Discovery Protocol (IEEE 802.1AB). It groups nodes
that report the same switch chassis identifier into one closest-first fabric
tier.

Use **`lldp-bm`** for bare-metal Slurm deployments and **`lldp-k8s`** for
Kubernetes deployments. Use **`lldp-sim`** to replay a saved
`lldpctl -f json` document without querying hosts or an `lldpd` socket.

## Scope and limitations

The provider intentionally reports leaf-switch locality only:

```text
compute node -> directly connected leaf switch
```

An LLDP advertisement received by a host does not contain the leaf-to-spine or
spine-to-core neighbor tables. Consequently, this provider cannot produce a
multi-tier fabric or reconstruct redundant switch paths. When authoritative
multi-tier topology is required, use an alternative provider designed for the
specific switch vendor or network fabric, such as the
[InfiniBand provider](./infiniband.md).

If the selected node interfaces report more than one distinct chassis ID,
generation fails instead of choosing an arbitrary path. Set `interfaces` or
`interfaceRegex` to the data-plane interfaces that should represent scheduling
locality. The two parameters are mutually exclusive. Multiple selected
interfaces attached to the same chassis are collapsed into one leaf
association.

## Output

The LLDP chassis ID is the stable identity used for grouping. MAC chassis IDs
are normalized to `lldp-<lowercase hex>` (for example,
`lldp-001122334455`). Other chassis-ID subtypes are represented by a stable
`lldp-<16-hex-digit>` SHA-256 prefix. This keeps generated switch identifiers
safe for Kubernetes labels and Slurm switch names.

With the Kubernetes engine, a node connected to chassis
`00:11:22:33:44:55` receives:

```yaml
fabric.topograph.run/tier-0: lldp-001122334455
```

The provider does not produce accelerator domains.

## `lldp-bm` (bare metal)

### Prerequisites

- `pdsh` installed on the Topograph host and configured for SSH access to every
  participating compute node
- `lldpd` running on every compute node and receiving LLDP advertisements from
  the connected switches
- `lldpctl` available in the remote nodes' `PATH`
- LLDP enabled on the relevant switch ports

The provider runs `lldpctl -f json` remotely through `pdsh`. Instance IDs and
node names are identical, matching the existing bare-metal provider model.

### Configuration

```yaml
provider: lldp-bm
engine: slurm
```

To restrict collection to selected data-plane interfaces:

```json
{
  "provider": {
    "name": "lldp-bm",
    "params": {
      "interfaces": ["eno1"]
    }
  },
  "engine": {
    "name": "slurm"
  }
}
```

Alternatively, select interfaces with a Go regular expression:

```json
{
  "provider": {
    "name": "lldp-bm",
    "params": {
      "interfaceRegex": "^eth_r[0-9]+_p[0-9]+$"
    }
  },
  "engine": {
    "name": "slurm"
  }
}
```

## `lldp-k8s` (Kubernetes)

### Prerequisites

- `lldpd` running on each Kubernetes host
- LLDP enabled on the relevant switch ports
- The host's `lldpd` control socket mounted into the node-data-broker container
  at `/var/run/lldpd.socket`
- The broker process permitted to open that socket

The Topograph image includes `lldpctl`. The broker queries the mounted host
socket once when its pod starts and writes the chassis ID to the
`topograph.run/lldp-chassis-id` node annotation. Restart the broker pod
to refresh the annotation after a cabling or LLDP configuration change.

Use the shipped `charts/topograph/values.k8s.lldp-example.yaml` as a starting
point:

```yaml
provider:
  name: lldp-k8s
  params:
    interfaceRegex: '^eth_r([0-9]+)_p[0-9]+$'
    railID: 'rail$1'

engine:
  name: k8s

nodeDataBroker:
  volumeMounts:
    - name: lldpd-socket
      mountPath: /var/run/lldpd.socket
  volumes:
    - name: lldpd-socket
      hostPath:
        path: /var/run/lldpd.socket
        type: Socket
```

Many installations expose the socket only to root or to an `lldpd` group. The
example values run only the broker as root, without privilege escalation or
added capabilities. If the socket is group-readable, keep the hardened
non-root defaults and add the host's `lldpd` group ID through
`nodeDataBroker.podSecurityContext.supplementalGroups` instead.

Schedule the node-data-broker only on participating nodes when control-plane or
other cluster nodes do not run `lldpd`. Align `nodeDataBroker.nodeSelector`
with `provider.params.nodeSelector` and the engine's node selection; otherwise
an uncollectable broker pod remains unready and the node-observer correctly
waits instead of generating an incomplete topology.

### Parameters

| Field | Type | Default | Description |
|---|---|---|---|
| `interfaces` | `[]string` | all LLDP interfaces | Local data-plane interfaces eligible for leaf-switch discovery. For `lldp-k8s`, Helm includes this setting in the node-data-broker config. |
| `interfaceRegex` | `string` | unset | Go regular expression selecting eligible local data-plane interface names. Mutually exclusive with `interfaces`. |
| `railID` | `string` | unset | Kubernetes-only Go regular-expression replacement template applied to each interface selected by `interfaceRegex`, for example `rail$1`. The broker resolves each selected interface to its physical NIC through `/sys/class/net/<interface>/device`; the resulting physical-NIC-to-rails mapping is sorted, deduplicated, and written to the shared `topograph-nic-rails` ConfigMap. Requires `interfaceRegex`. |
| `nodeSelector` | `map[string]string` | all nodes | Kubernetes-only node selector used when reading annotated nodes. |

For example, `interfaceRegex: '^eth_r([0-9]+)_p[0-9]+$'` and
`railID: 'rail$1'` derive `rail0` from `eth_r0_p0` and `rail3` from
`eth_r3_p7`. Rail extraction does not alter the canonical graph: the selected
interfaces must still identify one switch chassis, which becomes the node's
closest fabric tier. Rail data does not become part of the graph or a Node
annotation. The node-data-broker merge-patches one entry per node into the
`topograph-nic-rails` ConfigMap in the Topograph namespace. Override its name
with `nodeDataBroker.nicRailsConfigMap.name`. For each selected Linux interface,
the broker reads `/sys/class/net/<interface>/device` and uses the basename of
that symlink target—normally a PCI BDF such as `0000:3b:00.0`—as the physical
NIC name. Rails from multiple interfaces belonging to the same device are
combined. Each ConfigMap value is JSON, so device names remain node-scoped:

```yaml
data:
  node-a: '{"0000:3b:00.0":["rail0","rail1"],"0000:af:00.0":["rail2"]}'
  node-b: '{"0000:3b:00.0":["rail0"]}'
```

The Helm chart mounts the host's `/sys/class/net` read-only into the
node-data-broker when rail discovery is configured. A selected virtual
interface without a `device` symlink causes collection to fail rather than
publishing an incorrect physical NIC identifier.

### Verification

Check the broker chassis annotation and generated tier label:

```bash
kubectl get nodes -o json | jq '.items[] | {
  name: .metadata.name,
  chassis: .metadata.annotations["topograph.run/lldp-chassis-id"],
  leaf: .metadata.labels["fabric.topograph.run/tier-0"]
}'
```

When `railID` is configured, inspect the NIC-to-rails mapping:

```bash
kubectl get configmap -n topograph topograph-nic-rails -o json |
  jq '.data | to_entries[] | {node: .key, nics: (.value | fromjson)}'
```

If the broker does not become ready, inspect its logs and verify both the host
socket path and permissions:

```bash
kubectl logs -n topograph -l app.kubernetes.io/name=node-data-broker
ls -l /var/run/lldpd.socket
```

## `lldp-sim` (saved output)

The simulator reads one raw `lldpctl -f json` document and applies its selected
switch attachment to every node in the topology request. It exercises the same
JSON parser, interface selection, chassis validation, switch-ID generation,
and canonical graph conversion as the live providers. It does not execute
`lldpctl` or write Kubernetes node annotations.

```json
{
  "provider": {
    "name": "lldp-sim",
    "params": {
      "lldpFileName": "tests/output/lldp/lldp.log",
      "interfaceRegex": "^eth_r([0-9]+)_p[0-9]+$"
    }
  },
  "engine": {
    "name": "slurm"
  }
}
```

`lldpFileName` is required and may be an absolute path or a path relative to
the Topograph process working directory. The simulator supports `interfaces`
and `interfaceRegex` with the same mutually exclusive behavior as the live
providers. Because one saved document represents one node-local LLDP view, all
requested nodes are assigned to the selected chassis. Use a fixture whose
selected interfaces report one chassis; ambiguous fixtures fail during
provider loading.
