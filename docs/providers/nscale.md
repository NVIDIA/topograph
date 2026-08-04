# Nscale Topology Provider

The `nscale` topology provider reads topology data from the Nscale Radar API and converts it into Topograph's canonical three-tier topology graph.

The provider uses two Nscale APIs:

- **Radar API**: returns each instance's network path via `GET /v1/topology`
- **Instance API**: returns instance metadata via `GET /v2/instances?organizationID=<org>&regionID=<region>`

The Radar response supplies the provider instance ID, switch path, and optional block ID. The Instance API response maps provider instance IDs to hostnames using `metadata.id` and `metadata.name`; this is used by the Slurm engine when Topograph discovers Slurm nodes automatically.

## When to Use This Provider

Use this provider for Nscale environments where Radar is the topology source. It is most commonly used with the Slurm engine to generate `topology.conf` from the current Slurm node list.

If the request payload supplies explicit `nodes`, Topograph uses those instance ID to node name mappings directly. If `nodes` is omitted and the Slurm engine is used, Topograph runs `scontrol show nodes -o`, asks the Nscale Instance API for the instance catalog in the configured region, and keeps entries whose `metadata.name` matches a Slurm node name.

## Prerequisites

- A Radar API endpoint reachable from the Topograph host
- An Instance API endpoint reachable from the Topograph host
- An Nscale organization ID
- An API token with permission to read topology and instance metadata
- The Nscale region ID for the cluster
- For Slurm auto-discovery, `scontrol` must be available to the Topograph process

## Credentials

| Field | Required | Description |
|---|---|---|
| `org` | Yes | Nscale organization ID |
| `token` | Yes | Bearer token used for Radar and Instance API requests |
| `region` | Required for Slurm auto-discovery | Nscale region ID used for Instance API lookup and Slurm region assignment |

Store credentials in a YAML file:

```yaml
org: <ORGANIZATION_ID>
token: <API_TOKEN>
region: <REGION_ID>
```

Reference that file from the Topograph config:

```yaml
credentialsPath: /etc/topograph/nscale-credentials.yaml
```

Credentials can also be supplied directly in the topology request payload under `provider.creds`.

## Parameters

| Field | Required | Description |
|---|---|---|
| `radarApiUrl` | Yes | Base URL for the Radar API, for example `https://radar.example.com` |
| `instanceApiUrl` | Yes | Base URL for the Instance API, for example `https://api.example.com` |
| `trimTiers` | No | Number of highest topology tiers to trim from output. Defaults to `0` |

The top-level Topograph `pageSize` setting controls pagination for the Radar topology request.

## Configuration

Example Topograph config for Slurm:

```yaml
http:
  port: 49021
  ssl: false

provider: nscale
engine: slurm

requestAggregationDelay: 15s
credentialsPath: /etc/topograph/nscale-credentials.yaml

providerParams:
  radarApiUrl: https://radar.example.com
  instanceApiUrl: https://api.example.com

engineParams:
  plugin: topology/tree
  topologyConfigPath: /etc/slurm/topology.conf
```

Example request payload:

```json
{
  "provider": {
    "name": "nscale",
    "creds": {
      "org": "<ORGANIZATION_ID>",
      "token": "<API_TOKEN>",
      "region": "<REGION_ID>"
    },
    "params": {
      "radarApiUrl": "https://radar.example.com",
      "instanceApiUrl": "https://api.example.com"
    }
  },
  "engine": {
    "name": "slurm",
    "params": {
      "plugin": "topology/tree"
    }
  }
}
```

If you already have the instance ID to hostname mapping, you can include it explicitly:

```json
{
  "provider": {
    "name": "nscale",
    "creds": {
      "org": "<ORGANIZATION_ID>",
      "token": "<API_TOKEN>",
      "region": "<REGION_ID>"
    },
    "params": {
      "radarApiUrl": "https://radar.example.com",
      "instanceApiUrl": "https://api.example.com"
    }
  },
  "engine": {
    "name": "slurm"
  },
  "nodes": [
    {
      "region": "<REGION_ID>",
      "instances": {
        "<INSTANCE_ID_1>": "node001",
        "<INSTANCE_ID_2>": "node002"
      }
    }
  ]
}
```

## How It Works

For each region in the compute instance list, the provider fetches topology pages from Radar:

```text
GET <radarApiUrl>/v1/topology?limit=<pageSize>&offset=<offset>
Authorization: Bearer <token>
X-Organization: <org>
X-Region: <region>
```

Each returned instance is translated as follows:

| Radar field | Topograph field |
|---|---|
| `instance_id` | Instance ID |
| `network_node_path[0]` | Core tier |
| `network_node_path[1]` | Spine tier |
| `network_node_path[2]` | Leaf tier |
| `block_id` | Accelerator / NVLink domain |

For Slurm auto-discovery, the provider also fetches instance metadata:

```text
GET <instanceApiUrl>/v2/instances?organizationID=<org>&regionID=<region>
Authorization: Bearer <token>
```

It builds the same map produced by:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$INSTANCE_API_URL/v2/instances?organizationID=$ORG&regionID=$REGION" \
  | jq -r '.[] | "\(.metadata.id)\t\(.metadata.name)"'
```

## Verifying the Output

First verify that the Instance API returns the hostnames Slurm knows:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "$INSTANCE_API_URL/v2/instances?organizationID=$ORG&regionID=$REGION" \
  | jq -r '.[] | "\(.metadata.id)\t\(.metadata.name)"'
```

Then trigger topology generation:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)
curl -s "http://localhost:49021/v1/topology?uid=$id"
```

For the Slurm engine, verify that the generated `topology.conf` contains the expected switch hierarchy or block topology for the Nscale instances.
