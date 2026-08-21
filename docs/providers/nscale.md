# Nscale Topology Provider

The `nscale` topology provider reads topology data from the Nscale Radar API and converts it into Topograph's canonical three-tier topology graph.

The provider uses three Nscale APIs:

- **Radar API**: returns each instance's network path via `GET /v1/topology`
- **Placements API**: lists the organization's placements in a region via `GET /api/v2/placements`
- **Placement Servers API**: returns server metadata for a placement via `GET /api/v2/placements/{placementID}/servers`

The Radar response supplies the provider instance ID, switch path, and optional block ID. For Slurm auto-discovery, the provider lists every placement for the configured organization and region, then queries the Placement Servers API for each one and merges the results into a single instance-ID-to-hostname map using `metadata.id` and `metadata.name`.

## When to Use This Provider

Use this provider for Nscale environments where Radar is the topology source. It is most commonly used with the Slurm engine to generate `topology.conf` from the current Slurm node list.

If the request payload supplies explicit `nodes`, Topograph uses those instance ID to node name mappings directly. If `nodes` is omitted and the Slurm engine is used, Topograph runs `scontrol show nodes -o`, lists the organization's placements in the configured region via the Nscale Placements API, and asks the Placement Servers API for the server catalog of each placement. When `scontrol` returns a non-empty node list, only entries whose `metadata.name` exactly matches a Slurm node name are kept; if the node list is empty, every placement-server mapping is kept.

## Prerequisites

- A Radar API endpoint reachable from the Topograph host
- A Placements / Placement Servers API endpoint reachable from the Topograph host
- An Nscale organization ID
- An API token with permission to read topology, placements, and placement server metadata
- The Nscale region ID for the cluster
- For Slurm auto-discovery, `scontrol` must be available to the Topograph process

## Credentials

| Field | Required | Description |
|---|---|---|
| `org` | Yes | Nscale organization ID |
| `token` | Yes | Bearer token used for Radar, Placements, and Placement Servers API requests |
| `region` | Required for Slurm auto-discovery | Nscale region ID used for Slurm region assignment and to scope the Placements API listing |

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
| `instanceApiUrl` | Yes | Base URL for the Placements and Placement Servers APIs, for example `https://api.example.com` |
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

For Slurm auto-discovery, the provider first lists the organization's placements in the configured region from the Placements API:

```text
GET <instanceApiUrl>/api/v2/placements?organizationID=<org>&regionID=<region>
Authorization: Bearer <token>
```

The response is an array of placement objects; the provider extracts `metadata.id` from each entry. It then fetches server metadata from the Placement Servers API for every placement ID returned:

```text
GET <instanceApiUrl>/api/v2/placements/<placementId>/servers
Authorization: Bearer <token>
```

The response is an array of placement server objects. The provider extracts `metadata.id` (the server's unique identifier) and `metadata.name` (the hostname) from each entry and merges them across all placements into a single instance-ID-to-hostname map.

It builds the same map produced by:

```bash
set -euo pipefail

placement_ids=$(curl --fail --show-error --silent -H "Authorization: Bearer $TOKEN" \
  "$INSTANCE_API_URL/api/v2/placements?organizationID=$ORG_ID&regionID=$REGION_ID" \
  | jq -er '.[] | select(.metadata.id != "") | .metadata.id')

for placement_id in $placement_ids; do
  curl --fail --show-error --silent -H "Authorization: Bearer $TOKEN" \
    "$INSTANCE_API_URL/api/v2/placements/$placement_id/servers" \
    | jq -er '.[] | select(.metadata.id != "" and .metadata.name != "") | "\(.metadata.id)\t\(.metadata.name)"'
done
```

## Verifying the Output

When Slurm's node list (`scontrol show nodes -o`) is non-empty, `Instances2NodeMap`
only keeps a Placement Server entry when its `metadata.name` is an exact match for a
Slurm node name — there is no fuzzy or partial matching. If the node list is empty,
no filtering is applied and every placement-server mapping is retained. Before
triggering topology generation, compare the hostnames returned by the Placements and
Placement Servers APIs against Slurm's own node list and fail if they differ:

```bash
set -euo pipefail

slurm_nodes=$(scontrol show nodes -o | grep -oE 'NodeName=[^ ]+' | cut -d= -f2 | sort -u)
[ -n "$slurm_nodes" ] || { echo "FAIL: scontrol returned no nodes"; exit 1; }

placement_ids=$(curl --fail --show-error --silent -H "Authorization: Bearer $TOKEN" \
  "$INSTANCE_API_URL/api/v2/placements?organizationID=$ORG_ID&regionID=$REGION_ID" \
  | jq -er '.[] | select(.metadata.id != "") | .metadata.id')

placement_hostnames=$(for placement_id in $placement_ids; do
  curl --fail --show-error --silent -H "Authorization: Bearer $TOKEN" \
    "$INSTANCE_API_URL/api/v2/placements/$placement_id/servers" \
    | jq -er '.[] | select(.metadata.id != "" and .metadata.name != "") | .metadata.name'
done | sort -u)

if diff <(printf '%s\n' "$slurm_nodes") <(printf '%s\n' "$placement_hostnames"); then
  echo "OK: Placement Server hostnames match Slurm's node list"
else
  echo "FAIL: Placement Server hostnames differ from Slurm's node list"
  exit 1
fi
```

If the two lists differ, `Instances2NodeMap` will silently drop the mismatched nodes
from the generated topology rather than erroring, so this check should be run before
relying on Slurm auto-discovery.

Then trigger topology generation:

```bash
id=$(curl -s -X POST -H "Content-Type: application/json" -d @payload.json http://localhost:49021/v1/generate)
curl -s "http://localhost:49021/v1/topology?uid=$id"
```

For the Slurm engine, verify that the generated `topology.conf` contains the expected switch hierarchy or block topology for the Nscale instances.
