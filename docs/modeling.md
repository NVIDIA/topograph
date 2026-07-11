# Modeling and API Simulation

Topograph models are YAML files used to simulate discovered topology without querying a real cloud API, NetQ instance, InfiniBand fabric, or Kubernetes cluster. They are primarily used by tests and local development, but they are also useful when validating a scheduler integration against known topology shapes.

A model describes the same canonical topology that real providers eventually produce:

- A switch tree, used for Slurm `topology/tree` output and Kubernetes `leaf` / `spine` / `core` labels
- Node membership in hardware/connectivity blocks, used for block topology and optional accelerator labels
- Optional per-node labels used by provider simulations

Model loading lives in `pkg/models`. Model fixtures live under `tests/models/`.

## Where Models Are Used

Models are consumed in several simulation and local-development flows.

### KWOK Clusters

`kwok-nodes` renders virtual Kubernetes `Node` objects from a model file as a plain YAML manifest. The helper script `scripts/create-test-cluster.sh` then creates or reuses a local kind cluster, installs KWOK into it, and applies that manifest. This is useful when you want to run Topograph against a Kubernetes API without provisioning real nodes.

Prerequisites:

- `kind` installed and available on `PATH`
- `kubectl` installed and available on `PATH`
- A Docker-compatible runtime supported by kind
- Network access to GitHub releases when installing KWOK manifests

Build the manifest renderer:

```bash
make build
```

Render a KWOK node manifest from one of the embedded model fixtures:

```bash
bin/kwok-nodes -model medium.yaml -output /tmp/kwok-nodes.yaml
```

Create or reuse a kind cluster named `topograph`, install KWOK, and apply the generated nodes:

```bash
scripts/create-test-cluster.sh -m medium.yaml
```

Create or reuse a named kind cluster from an explicit model path, and keep the generated manifest:

```bash
scripts/create-test-cluster.sh \
  --cluster topo-demo \
  --model tests/models/nvl72.yaml \
  --output /tmp/nvl72-kwok-nodes.yaml \
  --gpus 8
```

To pass a kind cluster configuration file, add `--kind-config path/to/kind.yaml`. To pin KWOK installation to a release, add `--kwok-release vX.Y.Z`; otherwise the script uses GitHub's latest KWOK release download URL.

The utility uses the model-derived instance-to-hostname mapping, so model hostname `1101` becomes Kubernetes node `1101` with:

- `topograph.nvidia.com/instance: i-1101`
- `topograph.nvidia.com/region: <derived-region-or-none>`
- `kwok.x-k8s.io/node=fake` as both a label and annotation
- Model-derived labels such as `topology.kubernetes.io/region`, `topology.kubernetes.io/zone`, and `network.topology.nvidia.com/accelerator`

Generated Kubernetes node names come from model hostnames and are normalized to valid lowercase DNS names. For example, model hostname `I21` becomes Kubernetes node `i21`, while its generated instance ID `i-I21` is stored in `topograph.nvidia.com/instance`.

The script applies the manifest with kubeconfig context `kind-<cluster>`, matching the context name created by `kind create cluster --name=<cluster>`.

### Test Provider

The `test` provider simulates the Topograph API lifecycle itself. It can return successful topology output, delayed completion, malformed-request failures, provider failures, or a request that remains pending.

Use it when testing clients that call:

- `POST /v1/generate`
- `GET /v1/topology?uid=<request-id>`

For the complete API status-code simulation behavior, see [Test Mode and Test Provider](./providers/test.md).

### Provider Simulations

Several providers also have simulation variants, such as:

- `aws-sim`
- `gcp-sim`
- `oci-sim`
- `nebius-sim`
- `nscale-sim`
- `lambdai-sim`
- `dsx-sim`

These providers load a model file and then simulate that provider's API responses. This is useful when you want to exercise the normal provider translation logic without real provider credentials or infrastructure.

Simulation providers share these common parameters:

| Parameter | Required | Description |
|---|---:|---|
| `modelFileName` | Yes | Model file to load. A basename such as `medium.yaml` is loaded from `tests/models/`; absolute and relative paths are also supported. |
| `api_error` | No | Provider-specific test hook used by unit tests to simulate API failures. |
| `trimTiers` | No | Number of topology tiers to trim where supported by the simulated provider. |

Example request:

```json
{
  "provider": {
    "name": "aws-sim",
    "params": {
      "modelFileName": "medium.yaml"
    }
  },
  "engine": {
    "name": "slurm",
    "params": {
      "plugin": "topology/block"
    }
  }
}
```

## Model File Shape

A model usually has one required top-level section and one optional topology section:

```yaml
blocks:
  - ...
switches:
  ...
```

`switches` is a map and `blocks` is a list. `blocks[].nodes` is where model files declare compute node names; it creates the node records, applies block labels, and optionally attaches those nodes to a leaf switch through `blocks[].switch`. `switches` may be omitted for block-only models.

## Switches

The `switches` map describes the network hierarchy. Each key is the switch ID. Each value may contain:

| Field | Description |
|---|---|
| `labels` | Labels inherited by descendant nodes. Common keys are `topology.kubernetes.io/region` and `topology.kubernetes.io/zone`. |
| `switches` | Child switch IDs. |

Example:

```yaml
switches:
  core:
    labels:
      topology.kubernetes.io/region: us-west
    switches: [spine]
  spine:
    labels:
      topology.kubernetes.io/zone: zone1
    switches: [leaf1, leaf2]
  leaf1: {}
  leaf2: {}
```

Switch rules:

- A switch can have at most one parent switch.
- If a block names a switch with `blocks[].switch`, that block's `nodes` are attached to the switch before switch validation runs.

## Blocks

The `blocks` list describes sets of compute instances with similar hardware and connectivity characteristics. Each entry may contain:

| Field | Description |
|---|---|
| `switch` | Optional leaf switch ID. When set, this block's `nodes` are attached to that switch. |
| `nodes` | Required non-empty list of hostnames in this block. Compact ranges are supported. The model-backed test provider generates each instance ID by prefixing the hostname with `i-`. |
| `labels` | Optional labels applied to nodes generated from this block. For example, `network.topology.nvidia.com/accelerator` can identify an NVLink / accelerator domain. |

Example:

```yaml
blocks:
- switch: leaf1
  nodes: ["n[1-2]"]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
```

Block rules:

- The `blocks` section is the only place model files declare compute node names.
- Each block entry must declare at least one node.
- `switch` is optional. When set, it must reference an existing switch.
- `blocks[].nodes` creates node entries automatically.

## Compact Ranges

Model node lists support compact ranges:

```yaml
blocks:
- nodes: ["n[1-4]", "gpu[001-004]", node9]
```

These expand to:

```text
n1, n2, n3, n4, gpu001, gpu002, gpu003, gpu004, node9
```

Ranges are accepted in:

- `blocks[].nodes`

## Derived Data

After YAML parsing, Topograph completes the model before simulation uses it:

- Capacity block node ranges are expanded.
- Block `switch` references attach block nodes to switches.
- Switch names are copied from their map keys.
- Nodes are created from `blocks[].nodes`.
- Node `NetLayers` is derived from the switch path from leaf to root.
- Node labels are built by merging labels from the switch path and block labels.
- `Instances` is derived from node names and grouped by `labels.topology.kubernetes.io/region`; nodes without a region use `none`.

These derived fields are not written in YAML.

## Complete Examples

### Blocks With Switches

This model creates nodes from block membership and attaches them to a leaf switch.

```yaml
switches:
  core:
    switches: [leaf]
  leaf: {}

blocks:
- switch: leaf
  nodes: ["n[1-2]"]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
- switch: leaf
  nodes: [n3]
  labels:
    network.topology.nvidia.com/accelerator: nvl2
```

After loading:

- `n1`, `n2`, and `n3` are hostnames mapped from instance IDs `i-n1`, `i-n2`, and `i-n3`
- `n1` and `n2` belong to the first block and have `network.topology.nvidia.com/accelerator: nvl1` label
- `n3` belongs to the second block and has `network.topology.nvidia.com/accelerator: nvl2` label
- All three nodes have network layers `[leaf, core]`

### Blocks Without Switches

This model omits `switches`. Nodes are still created, block labels are still applied, and generated instances have no network layers.

```yaml
blocks:
- nodes: ["n[1-2]"]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
```

After loading:

- `n1` and `n2` belong to the first block
- `n1` and `n2` have `network.topology.nvidia.com/accelerator: nvl1` label
- `n1` and `n2` have no network layers

## Simulating the API

To simulate the Topograph API lifecycle, configure the `test` provider:

```yaml
http:
  port: 49021
  ssl: false

provider: test
engine: slurm

requestAggregationDelay: 2s
```

Then submit a request that names a model:

```json
{
  "provider": {
    "name": "test",
    "params": {
      "generateResponseCode": 202,
      "topologyResponseCode": 200,
      "modelFileName": "small-tree.yaml"
    }
  },
  "engine": {
    "name": "slurm"
  }
}
```

Expected flow:

1. `POST /v1/generate` returns `202 Accepted` and a request ID.
2. `GET /v1/topology?uid=<request-id>` returns `202 Accepted` while the request is queued or processing.
3. When processing completes, `/v1/topology` returns `200 OK` with the selected engine output.

To simulate API failures, set `generateResponseCode`, `topologyResponseCode`, and `errorMessage` in `provider.params`. For example:

```json
{
  "provider": {
    "name": "test",
    "params": {
      "generateResponseCode": 202,
      "topologyResponseCode": 500,
      "errorMessage": "simulated provider failure"
    }
  },
  "engine": {
    "name": "slurm"
  }
}
```

## Choosing the Right Simulation Path

Use the `test` provider when you want to validate API-client behavior:

- Request IDs
- Polling
- Pending responses
- Error status codes
- Retry behavior

Use a `*-sim` provider when you want to validate provider-specific topology translation:

- AWS, GCP, OCI, Nebius, Nscale, Lambda AI, or DSX topology paths
- Pagination behavior in simulated provider APIs
- Engine output generated from provider-shaped data
- Tree and block topology output from the same model

## Validation Checklist

Before using a new model in a regression test:

- Confirm every switch child has only one parent.
- Confirm every block `switch` reference points at an existing switch.
- Confirm no node appears under two blocks.
- Run the relevant provider simulation test or API flow with the target engine.
