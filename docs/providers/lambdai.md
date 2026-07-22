# Lambda Topology Provider

The `lambdai` topology provider reads topology data from the Lambda topology API and converts it into Topograph's canonical three-tier topology graph.

The provider queries a single endpoint, `GET /api/v1/topology/instance`, which returns each instance's network switch path and (when available) its NVLink domain. From this it builds a switch tree (for Slurm `topology/tree` or Kubernetes labels) and, when NVLink data is present, an NVLink domain map (for `topology/block`).

## When to Use This Provider

Use this provider for Lambda Cloud clusters where the Lambda topology API is the topology source. It works with both the Slurm engine (generating `topology.conf`) and the Kubernetes engine (labeling nodes).

With the **Slurm engine**, `lambdai` does **not** auto-discover nodes: the topology request must supply explicit `nodes` — a per-region map of provider instance IDs to hostnames (see [Configuration](#configuration)). With the **Kubernetes engine**, node discovery is automatic via the node-data-broker (see [Kubernetes engine](#kubernetes-engine)). Either way, each region triggers one paginated API call.

## Prerequisites

- A Lambda topology API endpoint reachable from the Topograph host
- A Lambda workspace ID
- An API token with permission to read instance topology
- The region ID for each cluster you query

## Credentials

| Field | Required | Description |
|---|---|---|
| `workspaceId` | Yes | Lambda workspace ID; sent as the `workspace_id` query parameter |
| `token` | Yes | Bearer token used for topology API requests |

Store credentials in a YAML file:

```yaml
workspaceId: <WORKSPACE_ID>
token: <API_TOKEN>
```

Reference that file from the Topograph config:

```yaml
credentialsPath: /etc/topograph/lambdai-credentials.yaml
```

Credentials can also be supplied directly in the topology request payload under `provider.creds`.

## Parameters

| Field | Required | Description |
|---|---|---|
| `url` | Yes | Base URL for the Lambda topology API, for example `https://cloud.example.com` |
| `trimTiers` | No | Number of highest topology tiers to trim from output. Defaults to `0` |

The region is **not** a parameter — it is taken from each entry in the request's `nodes` list and forwarded to the API as the `region` query parameter (the API requires it). The top-level Topograph `pageSize` setting controls the page size for paginated topology requests.

## Configuration

Example Topograph config for Slurm:

```yaml
http:
  port: 49021
  ssl: false

provider: lambdai
engine: slurm

requestAggregationDelay: 15s
pageSize: 200
credentialsPath: /etc/topograph/lambdai-credentials.yaml

providerParams:
  url: https://cloud.example.com

engineParams:
  plugin: topology/tree
  topologyConfigPath: /etc/slurm/topology.conf
```

Example request payload. The `nodes` list is required: each region maps provider instance IDs to the hostnames Topograph should emit.

```json
{
  "provider": {
    "name": "lambdai",
    "creds": {
      "workspaceId": "<WORKSPACE_ID>",
      "token": "<API_TOKEN>"
    },
    "params": {
      "url": "https://cloud.example.com"
    }
  },
  "engine": {
    "name": "slurm",
    "params": {
      "plugin": "topology/tree"
    }
  },
  "nodes": [
    {
      "region": "<REGION>",
      "instances": {
        "<INSTANCE_ID_1>": "node001",
        "<INSTANCE_ID_2>": "node002"
      }
    }
  ]
}
```

## How It Works

For each region in the request's `nodes` list, the provider pages through the topology endpoint:

```text
GET <url>/api/v1/topology/instance?workspace_id=<workspaceId>&region=<region>&page_size=<pageSize>
Authorization: Bearer <token>
```

The response is an envelope containing a `data` array and a pagination cursor:

```json
{
  "data": [
    { "id": "<instance-id>", "networkPath": [{ "id": "<switch>" }, { "id": "<switch>" }], "nvlink": null }
  ],
  "page_token": null
}
```

When `page_token` is non-null, the provider requests the next page with `&page_token=<token>` and repeats until it is null.

Each returned instance is translated as follows:

| API field | Topograph field |
|---|---|
| `id` | Instance ID (matched against the request's instance-to-hostname map) |
| `networkPath[0].id` | Leaf tier |
| `networkPath[1].id` | Spine tier |
| `networkPath[2].id` | Core tier |
| `nvlink.domain_id` + `nvlink.clique_id` | Accelerator / NVLink domain (`<domain_id>.<clique_id>`) |

`networkPath` is ordered from the leaf tier upward; paths shorter than three hops simply omit the higher tiers, and longer paths are logged and ignored.

NVLink domain data is best-effort. When the API returns `nvlink` for an instance, the provider derives its accelerator/NVLink domain, which enables `topology/block` output. When `nvlink` is null or absent, the provider emits the switch tree only. The exact shape of populated `nvlink` data may evolve; verify `topology/block` output once the API returns NVLink domains for your fleet.

## Kubernetes engine

With the `k8s` engine you do not pass `nodes` explicitly. Instead, the node-data-broker init container stamps each node with the two annotations the engine groups by, derived from fields the `lambda-cloud-controller` already sets on the Node object:

| Node field (set by `lambda-cloud-controller`) | Topograph annotation (set by node-data-broker) |
|---|---|
| `.spec.providerID` — `lambda://<instance-id>` | `topograph.nvidia.com/instance` — `<instance-id>` (matches the API `id` 1:1) |
| `topology.kubernetes.io/region` label — e.g. `stg-sjc01-cl03` | `topograph.nvidia.com/region` |

The Kubernetes engine then discovers nodes from these annotations, the provider queries the Lambda API once per region, and the engine writes `network.topology.nvidia.com/*` labels. The Node Observer re-triggers generation when nodes change.

Requirements:

- The `lambda-cloud-controller` must populate `.spec.providerID` and the `topology.kubernetes.io/region` label. The node-data-broker's init container errors and is retried by Kubernetes until both are present, so a node that is still initializing is simply labeled once its controller has finished.
- Keep the node-data-broker enabled (the chart default) — it is what translates the Node fields into the canonical annotations.
- **Node Observer trigger** — the Node Observer needs a watch selector or it crash-loops with `must specify nodeSelector and/or podSelector in trigger`. Set `nodeObserver.topograph.trigger.nodeSelector` (or `podSelector`) — e.g. `kubernetes.io/os: linux` to watch all nodes.
- **Tainted (GPU) nodes** — the node-data-broker is a DaemonSet and needs a matching toleration to run on tainted nodes (Lambda GPU instances carry `nvidia.com/gpu=true:NoSchedule`); without it those nodes are never annotated or labeled. `nodeDataBroker.tolerations[0].operator=Exists` lets it run on every node.
- **Image architecture** — the image must match the node architecture. Lambda GPU instances such as GH200 are `arm64`, so use a multi-arch or arm64 image.
- **Registry pull** — if the cluster cannot pull the image anonymously, create a pull secret and set the shared `imagePullSecrets` value.

Install with Helm (see the [Kubernetes quickstart](../get-started/quickstart-k8s.md) for the full flow):

```bash
# creds.yaml contains: workspaceId: <...>  /  token: <...>
kubectl create secret generic lambdai-creds \
  --from-file=credentials.yaml=creds.yaml -n topograph

helm install topograph oci://ghcr.io/nvidia/topograph/topograph \
  --version <chart-version> -n topograph --create-namespace \
  --set provider.name=lambdai \
  --set provider.params.url=https://cloud.example.com \
  --set engine.name=k8s \
  --set config.credentialsSecret=lambdai-creds \
  --set "nodeObserver.topograph.trigger.nodeSelector.kubernetes\.io/os=linux" \
  --set "nodeDataBroker.tolerations[0].operator=Exists"

# After a few seconds, topology labels appear on nodes:
kubectl get nodes --show-labels | grep network.topology.nvidia
```

Only fabric `tier-N` labels appear until the API returns `nvlink` data (see the note above); the accelerator label follows once it does.

## Verifying the Output

First sanity-check the API directly:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$URL/api/v1/topology/instance?workspace_id=$WORKSPACE_ID&region=$REGION" | jq .
```

Then trigger topology generation and read the result:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)
curl -s "http://localhost:49021/v1/topology?uid=$id"
```

For the Slurm engine, verify the generated `topology.conf` reflects the expected switch hierarchy for your Lambda instances. See the [Slurm engine documentation](../engines/slurm.md) for details.

## Simulation

A `lambdai-sim` provider variant is registered for testing without a live API. Instead of calling the topology API, it reads a YAML simulation model and serves it through the same translation path. Select it with `provider: lambdai-sim` and point it at a model file via the `modelFileName` parameter; see [Test Mode and Test Provider](./test.md) for the model-file format and simulation parameters.
