# topograph

A Helm chart for deploying [topograph](https://github.com/NVIDIA/topograph) on Kubernetes.

Topograph discovers the physical network topology of a cluster (NVLink domains, InfiniBand / Ethernet switch fabric, cloud rack topology) and exposes it to workload schedulers — Slurm, Kubernetes, and Slurm-on-Kubernetes (Slinky) — by applying node labels and/or writing scheduler-specific topology configuration.

## Prerequisites

- **Helm**: 3.8+ or 4.x. The chart has been verified against Helm 3.20.0 and Helm 4.1.4, with byte-identical `helm template` output under both.
- **Kubernetes**: no hard floor is declared by this chart; the rendered manifests use only `apps/v1`, `rbac.authorization.k8s.io/v1`, and `v1`, all stable since Kubernetes 1.9.
- **Provider-specific prerequisites** vary (CSP credentials, node labels, local binaries like `ibnetdiscover`, etc.). See `docs/providers/<name>.md` in the main repository for each provider's setup.

## Installation

From the OCI registry (recommended):

```bash
helm install topograph \
  oci://ghcr.io/nvidia/topograph/topograph \
  --version <chart-version> \
  --namespace topograph --create-namespace
```

From a local source checkout:

```bash
helm install topograph charts/topograph \
  --namespace topograph --create-namespace
```

To see available chart versions:

```bash
helm show chart oci://ghcr.io/nvidia/topograph/topograph
```

## Configuration

The default `values.yaml` ships the `test` provider + `k8s` engine, suitable for a smoke-test install. Production installs select a real provider:

```yaml
global:
  provider:
    name: dra        # or aws, gcp, oci, nebius, netq, infiniband-k8s, ...
  engine:
    name: k8s        # or slurm, slinky
```

For the full list of values and their defaults, see [`values.yaml`](./values.yaml). Example values files for specific deployment patterns:

- [`values.k8s-ib-example.yaml`](./values.k8s-ib-example.yaml) — InfiniBand provider on Kubernetes
- [`values.k8s-gcp-service-account-example.yaml`](./values.k8s-gcp-service-account-example.yaml) — GCP provider with a service account key mounted from a Secret
- [`values.k8s-gcp-federated-workload-identity-example.yaml`](./values.k8s-gcp-federated-workload-identity-example.yaml) — GCP provider using Workload Identity Federation
- [`values-slinky-tree-example.yaml`](./values-slinky-tree-example.yaml), [`values-slinky-block-example.yaml`](./values-slinky-block-example.yaml), [`values-slinky-partition-example.yaml`](./values-slinky-partition-example.yaml) — Slinky engine variants

### Values validation

The chart ships a [`values.schema.json`](./values.schema.json) that validates the most error-prone fields at install time — the `global.provider.name` and `global.engine.name` enums, type and range constraints on `replicaCount`, `image.pullPolicy`, `service.type`, `service.port`, and `verbosity`, and the expected shapes of `ingress`, `serviceMonitor`, and related nested objects. Invalid values are rejected by `helm install` and `helm template` with a clear schema-validation error.

The schema is deliberately narrow: per-provider credential requirements (which fields a given provider needs in its credentials map) are documented in prose in `docs/providers/<name>.md` rather than enforced in the schema, because the credential field sets evolve with upstream provider changes and are hard to keep accurate in a schema.

## Testing

Once installed, verify the deployment is functional with `helm test`:

```bash
helm test topograph --namespace topograph
```

The chart ships two `helm test` hook pods:

- **`test-healthz`** — probes the Topograph API's `/healthz` endpoint via the in-cluster Service and expects HTTP 200
- **`test-metrics`** — probes `/metrics`, expects HTTP 200 plus the `topograph_version` Prometheus metric in the response body (sanity check that the Prometheus registry is wired up)

Both test pods are removed automatically on success (`helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded`). On failure the pod persists so operators can inspect logs; the next `helm test` invocation replaces it.

### Air-gapped environments

By default, the test pods reuse the main topograph image. Topograph's default image is Alpine-based and ships with busybox `wget`, which the test probes use — so `helm test` works without pulling any additional image, including in air-gapped environments where only mirrored images are reachable.

If you run a topograph image variant without busybox `wget` (for example, the IB variant built on `ubuntu`), override the test image to point at one that does, via `tests.image.repository` and `tests.image.tag`. You can also disable the tests entirely with `tests.enabled=false`.

## Subcharts

The chart depends on two subcharts, both managed as local file dependencies:

- **`node-data-broker`** — DaemonSet that collects per-node attributes (NVLink clique IDs, etc.) as node annotations for the Kubernetes engine
- **`node-observer`** — watches node status changes and triggers topology regeneration

Both are installed together when you install this chart. Their values are accessible under the top-level keys `node-data-broker` and `node-observer` (enabled by default).

## References

- **Project documentation site**: <https://topograph.docs.buildwithfern.com/topograph>
- **Main repository**: <https://github.com/NVIDIA/topograph>
- **Provider-specific setup**: `docs/providers/` in the main repository
- **Engine documentation**: `docs/engines/k8s.md`, `docs/engines/slinky.md`, `docs/engines/slurm.md`
- **Node-labels reference**: `docs/reference/node-labels.md`
- **Contributing**: see [`CONTRIBUTING.md`](https://github.com/NVIDIA/topograph/blob/main/CONTRIBUTING.md) in the main repository

## License

Apache License 2.0. See [`LICENSE`](https://github.com/NVIDIA/topograph/blob/main/LICENSE) in the main repository.
