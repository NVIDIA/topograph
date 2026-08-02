# LLDP Topology Provider

The LLDP provider discovers each compute node's directly connected Ethernet
switch using the Link Layer Discovery Protocol (IEEE 802.1AB). It groups nodes
that report the same switch chassis identifier into one closest-first fabric
tier.

Use **`lldp-bm`** for bare-metal Slurm deployments and **`lldp-k8s`** for
Kubernetes deployments.

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
generation fails instead of choosing an arbitrary path. Set `interfaces` to
the data-plane interface or interfaces that should represent scheduling
locality. Multiple selected interfaces attached to the same chassis are
collapsed into one leaf association.

## Output

The LLDP chassis ID is the stable identity used for grouping. MAC chassis IDs
are normalized to `lldp-<lowercase hex>` (for example,
`lldp-001122334455`). Other chassis-ID subtypes are represented by a stable
`lldp-<16-hex-digit>` SHA-256 prefix. This keeps generated switch identifiers
safe for Kubernetes labels and Slurm switch names.

With the Kubernetes engine, a node connected to chassis
`00:11:22:33:44:55` receives:

```yaml
network.topology.nvidia.com/tier-0: lldp-001122334455
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

## `lldp-k8s` (Kubernetes)

### Prerequisites

- `lldpd` running on each Kubernetes host
- LLDP enabled on the relevant switch ports
- The host's `lldpd` control socket mounted into the node-data-broker container
  at `/var/run/lldpd.socket`
- The broker process permitted to open that socket

The Topograph image includes `lldpctl`. The broker queries the mounted host
socket once when its pod starts and writes the chassis ID to the
`topograph.nvidia.com/lldp-chassis-id` node annotation. Restart the broker pod
to refresh the annotation after a cabling or LLDP configuration change.

Use the shipped `charts/topograph/values.k8s.lldp-example.yaml` as a starting
point:

```yaml
provider:
  name: lldp-k8s
  params:
    interfaces:
      - eno1

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
| `interfaces` | `[]string` | all LLDP interfaces | Local data-plane interfaces eligible for leaf-switch discovery. For `lldp-k8s`, Helm forwards this setting to the node-data-broker. |
| `nodeSelector` | `map[string]string` | all nodes | Kubernetes-only node selector used when reading annotated nodes. |

### Verification

Check the broker annotation and generated tier label:

```bash
kubectl get nodes -o json | jq '.items[] | {
  name: .metadata.name,
  chassis: .metadata.annotations["topograph.nvidia.com/lldp-chassis-id"],
  leaf: .metadata.labels["network.topology.nvidia.com/tier-0"]
}'
```

If the broker does not become ready, inspect its logs and verify both the host
socket path and permissions:

```bash
kubectl logs -n topograph -l app.kubernetes.io/name=node-data-broker
ls -l /var/run/lldpd.socket
```
