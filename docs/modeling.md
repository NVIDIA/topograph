# Modeling and API Simulation

Topograph models are YAML files used to simulate discovered topology without querying a real cloud API, NetQ instance, InfiniBand fabric, or Kubernetes cluster. They are primarily used by tests and local development, but they are also useful when validating a scheduler integration against known topology shapes.

A model describes the same canonical topology that real providers eventually produce:

- A switch tree, used for Slurm `topology/tree` output and Kubernetes `leaf` / `spine` / `core` labels
- Node membership in accelerated domains, used for block topology and accelerator labels
- Optional per-node attributes used by provider simulations

Model loading lives in `pkg/models`. Model fixtures live under `tests/models/`.

## Where Models Are Used

Models are consumed in two different simulation flows.

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

A model has three top-level sections:

```yaml
switches:
  ...
nodes:
  ...
capacity_blocks:
  ...
```

All three sections are maps. `nodes` and `capacity_blocks` are flexible: you can specify node membership in either section, and Topograph completes the missing side during model loading.

## Switches

The `switches` map describes the network hierarchy. Each key is the switch ID. Each value may contain:

| Field | Description |
|---|---|
| `metadata` | Key-value metadata inherited by descendant nodes. Common keys are `region`, `availability_zone`, and `group`. |
| `switches` | Child switch IDs. |
| `nodes` | Compute node names attached to this switch. Compact node ranges are supported. |

Example:

```yaml
switches:
  core:
    metadata:
      region: us-west
    switches: [spine]
  spine:
    metadata:
      availability_zone: zone1
    switches: [leaf1, leaf2]
  leaf1:
    metadata:
      group: cb1
    nodes: ["n[1-2]"]
  leaf2:
    metadata:
      group: cb2
    nodes: [n3]
```

Switch rules:

- A switch can have at most one parent switch.
- A node can be attached to at most one switch.
- If a switch references a node, that node must exist either in `nodes` or be generated from `capacity_blocks`.
- Switch `nodes` entries are expanded through the same compact range syntax used elsewhere.

## Nodes

The `nodes` map describes compute nodes directly. Each key is the node name. The value may contain:

| Field | Description |
|---|---|
| `name` | Optional. If set, it must match the map key. Usually omitted. |
| `capacity_block_id` | Optional accelerated domain ID. If set and `capacity_blocks` is omitted, Topograph creates the corresponding capacity block entry. |
| `attributes.nvlink` | Optional accelerated-domain / NVLink identifier. Used by block topology simulation paths. |
| `attributes.status` | Optional node status metadata. |
| `attributes.timestamp` | Optional timestamp metadata. |
| `attributes.gpus` | Optional GPU inventory details. |

Example:

```yaml
nodes:
  n1:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n2:
    attributes:
      nvlink: nvl1
```

Node rules:

- `capacity_block_id` is optional.
- Nodes without `capacity_block_id` are still valid compute nodes.
- If `capacity_block_id` is set and `capacity_blocks` is omitted, Topograph creates the capacity block and adds the node to it.
- If a node is listed under `capacity_blocks.<id>.nodes`, Topograph fills in the node's missing `capacity_block_id`.
- If both sides specify different capacity block IDs for the same node, model loading fails.

## Capacity Blocks

The `capacity_blocks` map describes accelerated domains. Each key is the capacity block ID. The value may contain:

| Field | Description |
|---|---|
| `nodes` | Optional list of node names in this capacity block. Compact ranges are supported. |
| `attributes.nvlink` | Optional NVLink / accelerator domain identifier applied to nodes generated from this capacity block, and to listed top-level nodes when provided. |

Example:

```yaml
capacity_blocks:
  cb1:
    nodes: ["n[1-2]"]
    attributes:
      nvlink: nvl1
  cb2: {}
```

Capacity block rules:

- The entire `capacity_blocks` section may be omitted.
- Individual capacity block entries may omit `nodes`.
- Capacity block entries with no corresponding nodes are allowed and preserved.
- If top-level `nodes` is omitted, `capacity_blocks.<id>.nodes` creates node entries automatically.
- If top-level `nodes` is present, `capacity_blocks.<id>.nodes` must reference nodes in the top-level `nodes` map.

## Compact Ranges

Model node lists support compact ranges:

```yaml
nodes: ["n[1-4]", "gpu[001-004]", node9]
```

These expand to:

```text
n1, n2, n3, n4, gpu001, gpu002, gpu003, gpu004, node9
```

Ranges are accepted in:

- `switches.<switch>.nodes`
- `capacity_blocks.<id>.nodes`

## Derived Data

After YAML parsing, Topograph completes the model before simulation uses it:

- Switch node ranges are expanded.
- Capacity block node ranges are expanded.
- Node names are copied from their map keys.
- Switch names are copied from their map keys.
- Missing nodes can be created from `capacity_blocks.<id>.nodes`.
- Missing capacity block entries can be created from node `capacity_block_id` values.
- Node `NetLayers` is derived from the switch path from leaf to root.
- Node `Metadata` is built by merging switch metadata along the same path.
- `Instances` is derived from node names and grouped by `metadata.region`; nodes without a region use `none`.

These derived fields are not written in YAML.

## Complete Examples

### Nodes From Capacity Blocks

This compact model omits the `nodes` section. Nodes are created from capacity block membership.

```yaml
switches:
  core:
    switches: [leaf]
  leaf:
    nodes: ["n[1-2]", n3]

capacity_blocks:
  cb1:
    nodes: ["n[1-2]"]
    attributes:
      nvlink: nvl1
  cb2:
    nodes: [n3]
    attributes:
      nvlink: nvl2
```

After loading:

- `n1` and `n2` belong to `cb1` and have `attributes.nvlink: nvl1`
- `n3` belongs to `cb2` and has `attributes.nvlink: nvl2`
- All three nodes have network layers `[leaf, core]`

### Capacity Blocks From Nodes

This model omits `capacity_blocks`. Topograph creates `cb1` from `n1.capacity_block_id`.

```yaml
nodes:
  n1:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n2:
    attributes:
      nvlink: nvl2
```

After loading:

- `cb1.nodes` contains `n1`
- `cb1.attributes.nvlink` is populated from `n1.attributes.nvlink`
- `n2` remains a valid node without capacity block membership

### Orphan Capacity Block

This is valid. It declares a capacity block that currently has no nodes.

```yaml
nodes:
  n1:
    capacity_block_id: cb1

capacity_blocks:
  cb1: {}
  cb2: {}
```

After loading:

- `cb1.nodes` contains `n1`
- `cb2` remains present with no nodes

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

- AWS, GCP, OCI, Nebius, Lambda AI, or DSX topology paths
- Pagination behavior in simulated provider APIs
- Engine output generated from provider-shaped data
- Tree and block topology output from the same model

## Validation Checklist

Before using a new model in a regression test:

- Confirm every switch child has only one parent.
- Confirm every switched node is defined in `nodes` or generated from `capacity_blocks`.
- Confirm no node appears under two switches.
- Confirm capacity block membership does not conflict with node `capacity_block_id`.
- Run the relevant provider simulation test or API flow with the target engine.
