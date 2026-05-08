# Topograph Graph Engine

The `graph` engine returns instance-oriented topology metadata as JSON. It is intended for clients that need per-instance GPU and placement context rather than scheduler-specific output such as `topology.conf`, Kubernetes node labels, or a Slinky ConfigMap.

The engine preserves the provider/engine boundary: providers still discover topology and optional instance metadata, carry it on the canonical topology graph, and the `graph` engine only formats those records.

## Output

By default, the generated JSON is returned in the `/v1/topology` response:

```json
{
  "instances": [
    {
      "id": "I21",
      "type": "H100",
      "network_layers": ["leaf-a", "spine-a"],
      "attributes": {
        "nvlink": "nvl-1",
        "gpu": {
          "status": "known",
          "collected_at": "2026-01-01T13:59:00.000Z",
          "gpus": [
            {
              "index": 0,
              "pci_bus_id": "00000000:0F:00.0",
              "uuid": "GPU-example",
              "model": "NVIDIA H100 SXM5 80GB",
              "memory_mib": 81920
            }
          ]
        }
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
