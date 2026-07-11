# Topograph Graph Engine

The `graph` engine returns instance-oriented topology labels as JSON. It is intended for clients that need per-instance placement and accelerator-domain context rather than scheduler-specific output such as `topology.conf`, Kubernetes node labels, or a Slinky ConfigMap.

The engine preserves the provider/engine boundary: providers still discover topology and optional instance labels, carry them on the canonical topology graph, and the `graph` engine only formats those records.

## Output

By default, the generated JSON is returned in the `/v1/topology` response:

```json
{
  "instances": [
    {
      "id": "I21",
      "network_layers": ["leaf-a", "spine-a"],
      "labels": {
        "nvidia.com/gpu.product": "H100",
        "network.topology.nvidia.com/accelerator": "nvl-1"
      }
    }
  ]
}
```

Set `engine.params.topologyConfigPath` to write the JSON to an existing validated path on the Topograph host. When `topologyConfigPath` is set, the HTTP result body is `OK`.

## Request

The engine needs the instance IDs to export. Supply `nodes` in the request, or use a provider that can supply compute instances directly. The initial implementation is covered by the `test` provider and model-backed simulation providers.

```json
{
  "provider": {
    "name": "test",
    "params": {
      "modelFileName": "small-tree.yaml"
    }
  },
  "engine": {
    "name": "graph",
    "params": {
      "topologyConfigPath": "topology.json"
    }
  }
}
```
