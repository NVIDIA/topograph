# Topograph Slinky Engine

## Overview

The **slinky engine** is Topograph's engine for SLURM clusters running on Kubernetes. It is designed to work with the [Slinky project](https://github.com/SlinkyProject/) - an open-source set of integration tools by SchedMD that brings SLURM capabilities into Kubernetes environments.

While the [Slinky project](https://slinky.ai) provides comprehensive SLURM-on-Kubernetes orchestration (operators, schedulers, exporters, etc.), Topograph's slinky engine complements this ecosystem by providing **topology discovery and configuration management** for SLURM clusters running in Kubernetes.

The slinky engine bridges the gap between Kubernetes infrastructure and SLURM workload management by updating SLURM topology configurations stored in Kubernetes ConfigMaps.

## How It Works

1. **Node Discovery**: Queries Kubernetes nodes and SLURM pods to build a topology map
2. **Topology Generation**: Creates SLURM topology configuration (tree or block format)
3. **ConfigMap Management**: Updates the specified ConfigMap with new topology data including metadata annotations for tracking and debugging

## Configuration

### Required Parameters

```json
{
  "engine": {
    "name": "slinky",
    "params": {
      "namespace": "ds-slurm",
      "pod_label": "app.kubernetes.io/component=compute",
      "topology_config_path": "topology.conf",
      "topology_configmap_name": "slurm-config"
    }
  }
}
```

### Optional Parameters

```json
{
  "engine": {
    "name": "slinky",
    "params": {
      "namespace": "ds-slurm",
      "pod_label": "app.kubernetes.io/component=compute",
      "topology_config_path": "topology.conf",
      "topology_configmap_name": "slurm-config",
      "plugin": "topology/block",
      "block_sizes": "8,16,32"
    }
  }
}
```

## ConfigMap Annotations

Slinky automatically adds metadata annotations to managed ConfigMaps for improved observability:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: slurm-config
  annotations:
    # Topograph metadata
    topograph.nvidia.com/engine: "slinky"
    topograph.nvidia.com/topology-managed-by: "topograph"
    topograph.nvidia.com/last-updated: "2024-01-01T10:11:00Z"
    topograph.nvidia.com/slurm-namespace: "slurm"
    topograph.nvidia.com/plugin: "topology/tree"
    topograph.nvidia.com/block-sizes: "8,16,32"

    # Original annotations preserved
    meta.helm.sh/release-name: slurm
    meta.helm.sh/release-namespace: slurm
data:
  topology.conf: |
    SwitchName=sw1 Switches=sw[2-3]
    SwitchName=sw2 Nodes=node[1-4]
    SwitchName=sw3 Nodes=node[5-8]
```

### Annotation Reference

| Annotation                                 | Description                               |
| ------------------------------------------ | ----------------------------------------- |
| `topograph.nvidia.com/engine`              | Engine that manages this ConfigMap        |
| `topograph.nvidia.com/topology-managed-by` | Indicates topograph manages topology data |
| `topograph.nvidia.com/last-updated`        | RFC3339 timestamp of last update          |
| `topograph.nvidia.com/slurm-namespace`     | SLURM cluster namespace                   |
| `topograph.nvidia.com/plugin`              | Topology plugin used (tree/block)         |
| `topograph.nvidia.com/block-sizes`         | Block sizes for block topology            |

## Usage Examples

### Basic Tree Topology

```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{
    "provider": {"name": "aws"},
    "engine": {
      "name": "slinky",
      "params": {
        "namespace": "ds-slurm",
        "pod_label": "app.kubernetes.io/component=compute",
        "topology_config_path": "topology.conf",
        "topology_configmap_name": "slurm-config"
      }
    }
  }' \
  http://localhost:49021/v1/generate
```

### Block Topology with Custom Sizes

```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{
    "provider": {"name": "aws"},
    "engine": {
      "name": "slinky",
      "params": {
        "namespace": "ds-slurm",
        "pod_label": "app.kubernetes.io/component=compute",
        "topology_config_path": "topology.conf",
        "topology_configmap_name": "slurm-config",
        "plugin": "topology/block",
        "block_sizes": "8,16,32"
      }
    }
  }' \
  http://localhost:49021/v1/generate
```
