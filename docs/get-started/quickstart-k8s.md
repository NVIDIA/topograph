# Install on Kubernetes

Topograph installs on a Kubernetes cluster via a Helm chart. The same chart supports two engines:

- **[`k8s` engine](#engine-k8s)** — labels Kubernetes nodes with topology keys so schedulers (native `podAffinity`, KAI Scheduler, Kueue TAS, etc.) can make topology-aware placement decisions
- **[`slinky` engine](#engine-slinky)** — writes Slurm topology configuration into a `ConfigMap` for [Slinky](https://github.com/SlinkyProject) (Slurm-on-Kubernetes) deployments

Prerequisites, install flow, and verification are common to both — the engines differ only in a few `global.engine.*` values and in what downstream artifact is produced.

## Prerequisites

- **Kubernetes**: 1.27 or later
- **Helm**: 3.8+ or 4.x
- **`kubectl`** with permission to install a chart and create a `Namespace`
- **A supported provider** for your environment — see the [provider documentation](../providers/) for per-provider setup (credentials, required cluster state, etc.)
- **For the `slinky` engine only**: a Slinky cluster already deployed in the target Kubernetes cluster — Topograph does not deploy Slinky itself

## Install

Base install command (pick the engine-specific flags from the two sections below):

```bash
helm install topograph \
  oci://ghcr.io/nvidia/topograph/topograph \
  --version <chart-version> \
  --namespace topograph --create-namespace \
  --set global.provider.name=<provider> \
  --set global.engine.name=<engine> \
  # ... engine-specific flags ...
```

Replace `<provider>` with one of the supported values (`aws`, `gcp`, `oci`, `nebius`, `netq`, `dra`, `infiniband-k8s`, `lambdai`, `cw`). To see available chart versions, run `helm show chart oci://ghcr.io/nvidia/topograph/topograph`.

Provider-specific credentials and parameters are passed via Helm values. See the [chart README](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/README.md) and [`values.yaml`](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/values.yaml) for the full values shape, plus the example values files shipped in the chart directory.

## Verify

The `helm test` verification is the same regardless of engine:

```bash
helm test topograph --namespace topograph
```

The bundled test hooks probe `/healthz` and `/metrics` inside the cluster and expect HTTP 200 plus the `topograph_version` metric in the response body. A green result confirms the API server is running.

Engine-specific verification (seeing the actual topology output) differs — see each engine's section below.

## Engine: `k8s` <a name="engine-k8s"></a>

Engine-specific install flag:

```bash
  --set global.engine.name=k8s
```

Nothing else is required — the `k8s` engine labels nodes directly from the default chart deployment. A few seconds after install, topology labels appear on cluster nodes:

```bash
kubectl get nodes --show-labels | grep network.topology.nvidia
```

If labels are missing, inspect the Topograph logs:

```bash
kubectl logs -n topograph -l app.kubernetes.io/name=topograph
```

## Engine: `slinky` <a name="engine-slinky"></a>

Engine-specific install flags point the `slinky` engine at the Slinky deployment:

```bash
  --set global.engine.name=slinky \
  --set global.engineParams.namespace=<slinky-namespace> \
  --set global.engineParams.topologyConfigmapName=<configmap-name> \
  --set global.engineParams.topologyConfigPath=topology.conf
```

The full engine-parameter shape (`podSelector`, `plugin`, `blockSizes`, per-partition topologies, …) is documented in the [chart README](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/README.md). Example values files for common Slinky scenarios ship in the chart directory:

- [`values.slinky.tree-example.yaml`](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/values.slinky.tree-example.yaml) — tree topology
- [`values.slinky.block-example.yaml`](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/values.slinky.block-example.yaml) — block topology
- [`values.slinky.partition-example.yaml`](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/values.slinky.partition-example.yaml) — per-partition topologies (Slurm 25.05+)

To confirm the topology was written to the target `ConfigMap`:

```bash
kubectl get configmap -n <slinky-namespace> <configmap-name> -o yaml
```

The key configured via `topologyConfigPath` (by default `topology.conf`) should contain the generated Slurm topology configuration.

## Where to go next

- **[Kubernetes engine reference](../engines/k8s.md)** — configuration, access patterns (`Ingress`, `HTTPRoute`, `NetworkPolicy`, `ServiceMonitor`), mixed workload considerations
- **[Slinky engine reference](../engines/slinky.md)** — `slinky` engine parameters, `ConfigMap` annotations, tree / block / per-partition usage examples (the chart-level deployment surface is shared with the `k8s` engine and is documented under the Kubernetes engine reference above)
- **[Chart README](https://github.com/NVIDIA/topograph/blob/main/charts/topograph/README.md)** — full values reference, `helm test` details, air-gapped environments, subchart layout
- **[Node labels reference](../reference/node-labels.md)** — label key semantics, value behavior (FNV hashing for long values), integration with the NVIDIA GPU Operator, downstream consumer notes (relevant primarily to the `k8s` engine)
- **[Provider documentation](../providers/)** — per-provider prerequisites and configuration
- **[Config and API reference](../api.md)** — `topograph-config.yaml` schema, API endpoints, request/response shape
- **[Architecture](../architecture.md)** — how the API server, Node Observer, Node Data Broker, Provider, and Engine fit together
