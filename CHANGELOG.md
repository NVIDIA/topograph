# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `govulncheck` job in the Go CI workflow for symbol-level vulnerability scanning on pull requests.
- OCI labels missing from `docker/metadata-action` on the Topograph container image: `org.opencontainers.image.documentation`, `authors`, and `vendor` ([#377](https://github.com/NVIDIA/topograph/pull/377)).
- Helm chart metadata: `home`, `icon`, `maintainers`, `keywords`, and Artifact Hub annotations ([#377](https://github.com/NVIDIA/topograph/pull/377)).
- Helm `env`, `initContainers`, and `lifecycle` overrides across the API server, node-observer, and node-data-broker containers.
- Lambda provider Kubernetes node-data-broker support: Topograph instance and region annotations are derived from Lambda node `.spec.providerID` and `topology.kubernetes.io/region`, enabling automatic node discovery with the Kubernetes engine ([#375](https://github.com/NVIDIA/topograph/pull/375)).

### Changed

- Simulation model node names are now treated as hostnames; the model-backed test provider generates their instance IDs with an `i-` prefix.
- The node-observer and node-data-broker are now rendered directly by the main Topograph Helm chart instead of local subcharts. Their existing `node-observer.*` and `node-data-broker.*` values paths are unchanged.
- **BREAKING (Helm chart `0.5.0` → `0.6.0`):** the chart now ships a hardened default security context across the API server, node-observer, and node-data-broker: non-root (`runAsNonRoot`, UID/GID `65532`), `seccompProfile: RuntimeDefault`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, and all capabilities dropped — satisfying the Kubernetes `restricted` Pod Security Standard out of the box. This changes the default runtime posture of every workload; operators who relied on root, a writable rootfs, or added capabilities must override the relevant keys (see the migration note below). `appVersion` is unchanged (`v0.5.0`; no binary change).
- Go toolchain bumped to **1.26.5** (`go.mod`, `Dockerfile`, CI) to address reachable stdlib vulnerabilities reported by `govulncheck`.
- Slinky partition discovery now prefers the Slinky controller pod and falls back to a login pod, so clusters without optional login pods can still discover partitions ([#362](https://github.com/NVIDIA/topograph/pull/362)).
- Slinky engine `useGpuCliqueLabel` now emits an actionable diagnostic when no block domains can be built: the error reports how many nodes were scanned and why each was skipped (no Slurm mapping, missing `nvidia.com/gpu.clique` label, or missing the node-data-broker-written `topograph.nvidia.com/instance` annotation), and lists the offending node names. When no Kubernetes nodes are selected at all, it reports a distinct error pointing at the engine `nodeSelector`.
- Simulation model YAML schema simplified: compute nodes are now declared through `blocks[].nodes`, with optional `blocks[].switch` attachment and `blocks[].labels`; the older separate `nodes` and `capacity_blocks` sections were removed from fixtures and docs ([#394](https://github.com/NVIDIA/topograph/pull/394)).

### Fixed

- Helm node-observer now targets the rendered Topograph Service fullname in `generateTopologyUrl`.
- Lambda provider client now matches the Lambda topology API request and response contract: required `region` query parameter, `{data, page_token}` envelope, `page_token` pagination, and `networkPath` object mapping ([#374](https://github.com/NVIDIA/topograph/pull/374)).
- Slinky engine now skips pods without a resolvable Slurm node name instead of adding an empty instance-to-node mapping ([#380](https://github.com/NVIDIA/topograph/pull/380)).

### Security

- Removed unused RBAC verbs from the Topograph API server and node-data-broker ClusterRoles (least-privilege): API server `pods` rule dropped `get` (list-only), `daemonsets` rule dropped `list` (get-only), and the Slinky `configmaps` rule dropped `list`; node-data-broker `nodes` rule dropped `list` (get/update), and the InfiniBand `daemonsets`/`pods` rules dropped `list`/`get` respectively (get-only, list-only).

- node-observer ClusterRole no longer grants unused `get`; `nodes` list/watch now gated on `trigger.nodeSelector`.
### Migration (Helm — hardened security context)

The chart's hardened defaults are a breaking change for two deployment shapes; override only the affected keys/component:

| If you run | Override |
|--------|----------|
| `infiniband-k8s` (broker reads `/sys/class`) | A **complete** privileged override on `node-data-broker` — `securityContext: { privileged: true, allowPrivilegeEscalation: true, readOnlyRootFilesystem: false, runAsNonRoot: false, runAsUser: 0 }` plus `podSecurityContext.runAsNonRoot: false`. A partial override (only `privileged: true`) is rejected at admission because the default `allowPrivilegeEscalation: false` remains. Both shipped IB examples (`values.k8s.ib-example.yaml` and `values.slinky.ib.block-example.yaml`) are updated to the complete form. |
| `engine: slurm` or `engine: graph` in-cluster (writes `topology.conf`) | `securityContext.readOnlyRootFilesystem: false` and a writable volume at the configured output path. |

The default `k8s`/`slinky` engines and all other providers need no change.

## [0.5.0] - 2026-06-30

### Added

- **Graph engine** (`engine: graph`) for canonical topology graph output ([#314](https://github.com/NVIDIA/topograph/pull/314)).
- **Nscale provider** for Nscale cloud topology discovery ([#239](https://github.com/NVIDIA/topograph/pull/239)).
- Helm **`namespace`** value to install all chart resources into a namespace other than the release namespace ([#345](https://github.com/NVIDIA/topograph/pull/345)).
- Helm **ConfigMap mounts** for the node-data-broker DaemonSet (`node-data-broker.configMapMounts`) ([#347](https://github.com/NVIDIA/topograph/pull/347)).
- Deployment **checksum annotation** so Topograph rolls when its ConfigMap changes ([#346](https://github.com/NVIDIA/topograph/pull/346)).
- **Empty block complementing** for block-topology output ([#343](https://github.com/NVIDIA/topograph/pull/343)).
- **Helm chart tests** (`make chart-test`) using helm-unittest ([#336](https://github.com/NVIDIA/topograph/pull/336), [#361](https://github.com/NVIDIA/topograph/pull/361)).
- Downstream packaging knobs for deb/rpm builds ([#333](https://github.com/NVIDIA/topograph/pull/333)).
- Fern docs **global NVIDIA theme** adoption ([#339](https://github.com/NVIDIA/topograph/pull/339)).
- Nscale provider documentation ([#326](https://github.com/NVIDIA/topograph/pull/326)).

- **node-data-broker** runs as the DaemonSet main container instead of an init container plus a `curlimages/curl` placeholder ([#368](https://github.com/NVIDIA/topograph/pull/368)). The `node-data-broker-initc` binary applies node annotations at startup, serves `/healthz`, and stays running until the pod receives SIGTERM.
- New `node-data-broker.port` Helm value (default `8080`) for the broker health HTTP server.
- New `node-data-broker.refreshInterval` Helm value (default `5m`) to re-apply node annotations periodically after startup. Set to `0` to disable periodic refresh.
- New `node-data-broker.startupProbe` settings (default `failureThreshold: 30`, `periodSeconds: 10`, i.e. a 5-minute startup budget) so slow providers such as InfiniBand `ibnetdiscover` can finish before liveness/readiness probes take effect.
- Startup, liveness, and readiness probes on the node-data-broker container, all targeting `/healthz`.
- New CLI flags on `node-data-broker-initc`: `--port` and `--refresh-interval`.

### Changed

- **Slinky engine**: `nvidia.com/gpu.clique` can override provider accelerator domains when present on a node ([#342](https://github.com/NVIDIA/topograph/pull/342)).
- **Kubernetes engine**: prefer the GPU clique label for accelerator domains when configured ([#341](https://github.com/NVIDIA/topograph/pull/341)).
- **Node observer** watches the Topograph API pod and triggers topology regeneration when the API becomes Ready after startup or a container restart ([#367](https://github.com/NVIDIA/topograph/pull/367)).
- Helm image tags default to the chart **`appVersion`** when unset ([#360](https://github.com/NVIDIA/topograph/pull/360)).
- Providers reuse the shared retrying HTTP helper; string-map config parsing replaced with mapstructure ([#356](https://github.com/NVIDIA/topograph/pull/356), [#355](https://github.com/NVIDIA/topograph/pull/355)).
- Node attribute handling simplified in the canonical graph ([#349](https://github.com/NVIDIA/topograph/pull/349)).
- Helm install docs updated to use the chart repository ([#357](https://github.com/NVIDIA/topograph/pull/357)).

- The node-data-broker DaemonSet image defaults to `ghcr.io/nvidia/topograph` instead of `curlimages/curl`.
- `node-data-broker-initc` reuses a single in-cluster Kubernetes clientset for the initial apply and all periodic refreshes.
- InfiniBand provider documentation updated for the flattened Helm values layout.

### Fixed

- **Nebius provider**: read instance metadata from IMDS ([#353](https://github.com/NVIDIA/topograph/pull/353)).
- **Chart RBAC**: support pre-existing ServiceAccounts when RBAC creation is managed separately ([#364](https://github.com/NVIDIA/topograph/pull/364)).
- **API server**: preserve replacement timer in the trailing-delay request queue; snapshot queue completion results correctly ([#348](https://github.com/NVIDIA/topograph/pull/348), [#351](https://github.com/NVIDIA/topograph/pull/351)).
- Chart-test CI pins a compatible **Helm** version ([#359](https://github.com/NVIDIA/topograph/pull/359)).
- deb/rpm build scripts: quote paths derived from environment variables ([#334](https://github.com/NVIDIA/topograph/pull/334), [#335](https://github.com/NVIDIA/topograph/pull/335)).
- Fern docs CI and version-registration edge cases ([#338](https://github.com/NVIDIA/topograph/pull/338), [#327](https://github.com/NVIDIA/topograph/pull/327)).
- Minor Helm template fixes ([#331](https://github.com/NVIDIA/topograph/pull/331)).

### Removed

- The node-data-broker init container (`init-node-labels`), the `initc` values block, the `node-data-broker.initImage` Helm helper, and the `tail -f /dev/null` placeholder command.
- Dependency on the `curlimages/curl` image for the node-data-broker subchart.

### Migration (Helm — node-data-broker)

If you override node-data-broker settings today, update your values as follows:

| Before | After |
|--------|-------|
| `node-data-broker.initc.extraArgs` | `node-data-broker.extraArgs` |
| `node-data-broker.initc.image.*` | `node-data-broker.image.*` (now drives the sole container) |
| `node-data-broker.command` (`tail -f /dev/null`) | Remove — no longer needed |
| `node-data-broker.initc.enabled` | Remove — broker always runs when the subchart is enabled |

Example:

```yaml
# Before
node-data-broker:
  initc:
    extraArgs:
      - gpu-operator-namespace=my-namespace

# After
node-data-broker:
  extraArgs:
    - gpu-operator-namespace=my-namespace
  refreshInterval: 5m   # optional; default shown
```

**InfiniBand (`infiniband-k8s`) deployments** that override the broker image to `ghcr.io/nvidia/topograph/ib` for `ibnetdiscover` should continue to do so until IB tooling is folded into the main Topograph image.

[Full changelog](https://github.com/NVIDIA/topograph/compare/v0.4.0...v0.5.0)

---

## [0.4.0] - 2026-05-14

### Added

- **Nscale provider** ([#239](https://github.com/NVIDIA/topograph/pull/239)).
- **Gateway API** support via optional `HTTPRoute` template ([#276](https://github.com/NVIDIA/topograph/pull/276)).
- **Slinky dynamic nodes reconciliation** ([#241](https://github.com/NVIDIA/topograph/pull/241)).
- Slinky **`slurmConfigUpdateMode`** parameter ([#300](https://github.com/NVIDIA/topograph/pull/300)).
- Slinky **`podSelector`** support in partition topologies ([#295](https://github.com/NVIDIA/topograph/pull/295)).
- **DSX provider simulator** ([#287](https://github.com/NVIDIA/topograph/pull/287)).
- Simulation models refactored to support explicit and implicit node configuration ([#309](https://github.com/NVIDIA/topograph/pull/309), [#311](https://github.com/NVIDIA/topograph/pull/311)).
- **Versioned Fern documentation** with CI version stamping and frozen content at publish time ([#313](https://github.com/NVIDIA/topograph/pull/313), [#316](https://github.com/NVIDIA/topograph/pull/316)).
- Helm **`values.schema.json`**, **`helm test`** hook pods, and an expanded chart README ([#275](https://github.com/NVIDIA/topograph/pull/275)).
- Authoritative **node labels and annotations** reference ([#254](https://github.com/NVIDIA/topograph/pull/254)).
- **`make qualify`** pre-push aggregator (fmt, vet, lint, test) ([#256](https://github.com/NVIDIA/topograph/pull/256)).
- **`AGENTS.md`** and **`.claude/CLAUDE.md`** for AI coding agents ([#253](https://github.com/NVIDIA/topograph/pull/253)).
- Kubernetes and Slurm **get-started quickstarts** ([#292](https://github.com/NVIDIA/topograph/pull/292)).
- InfiniBand, NetQ, and DRA provider documentation ([#243](https://github.com/NVIDIA/topograph/pull/243)).
- `CODE_OF_CONDUCT`, `SECURITY.md`, and pull request template ([#273](https://github.com/NVIDIA/topograph/pull/273)).

### Changed

- Refactored the canonical **topology graph** to reduce complexity ([#306](https://github.com/NVIDIA/topograph/pull/306)).
- Removed obsolete **toposim** tooling and **protobuf** definitions ([#310](https://github.com/NVIDIA/topograph/pull/310)).
- Documentation reorganized into overview, providers, engines, and reference sections ([#267](https://github.com/NVIDIA/topograph/pull/267)).
- Helm chart declares **`kubeVersion: ">=1.27.0-0"`** on the umbrella chart and subcharts ([#291](https://github.com/NVIDIA/topograph/pull/291)).
- Go toolchain and dependencies updated ([#277](https://github.com/NVIDIA/topograph/pull/277)).
- Node observer **retries failed topology requests** after a configurable delay ([#242](https://github.com/NVIDIA/topograph/pull/242)).

### Fixed

- Topology spec output for the **switch hierarchy** ([#246](https://github.com/NVIDIA/topograph/pull/246)).
- Slinky partition discovery RBAC and **`useDynamicNodes`** interaction ([#319](https://github.com/NVIDIA/topograph/pull/319), [#320](https://github.com/NVIDIA/topograph/pull/320)).
- Slurm/Slinky topology edge cases ([#297](https://github.com/NVIDIA/topograph/pull/297)).
- Flatten multi-line provider error messages for logging ([#301](https://github.com/NVIDIA/topograph/pull/301)).
- Fern docs CI, preview artifacts, and custom-domain configuration ([#308](https://github.com/NVIDIA/topograph/pull/308), [#290](https://github.com/NVIDIA/topograph/pull/290), [#304](https://github.com/NVIDIA/topograph/pull/304)).
- Docker BuildKit proxy settings on NV GitHub runners ([#296](https://github.com/NVIDIA/topograph/pull/296)).

[Full changelog](https://github.com/NVIDIA/topograph/compare/v0.3.0...v0.4.0)

---

## [0.3.0] - 2026-03-24

### Added

- **Lambda provider** and simulator ([#198](https://github.com/NVIDIA/topograph/pull/198), [#200](https://github.com/NVIDIA/topograph/pull/200)).
- **Lookup endpoint** for topology queries ([#218](https://github.com/NVIDIA/topograph/pull/218)).
- **Request aggregation** keyed by payload hash (FNV-64) ([#217](https://github.com/NVIDIA/topograph/pull/217), [#219](https://github.com/NVIDIA/topograph/pull/219)).
- **NetQ provider**: programmatic OPID fetch and NVLink domain discovery ([#180](https://github.com/NVIDIA/topograph/pull/180), [#186](https://github.com/NVIDIA/topograph/pull/186)).
- **GCP provider**: external API authentication and **federated workload identity** for Kubernetes deployments ([#204](https://github.com/NVIDIA/topograph/pull/204), [#224](https://github.com/NVIDIA/topograph/pull/224)).
- **Slurm engine**: dynamic nodes support (later partially reverted) and flat YAML fallback when partition topology is absent ([#202](https://github.com/NVIDIA/topograph/pull/202), [#235](https://github.com/NVIDIA/topograph/pull/235)).
- **Kubernetes**: node selector for topology triggers, configurable topology node label names, **ServiceMonitor**, and InfiniBand example values ([#184](https://github.com/NVIDIA/topograph/pull/184), [#221](https://github.com/NVIDIA/topograph/pull/221), [#199](https://github.com/NVIDIA/topograph/pull/199), [#191](https://github.com/NVIDIA/topograph/pull/191)).
- **Integration test** payloads and harness ([#203](https://github.com/NVIDIA/topograph/pull/203), [#205](https://github.com/NVIDIA/topograph/pull/205)).
- **`/ib` container image** with InfiniBand diagnostic tools ([#190](https://github.com/NVIDIA/topograph/pull/190)).
- Default **provider and engine params** in Helm values ([#234](https://github.com/NVIDIA/topograph/pull/234)).
- Topology **upper-tier trimming** option ([#233](https://github.com/NVIDIA/topograph/pull/233)).
- Topograph **version label** on Prometheus metrics ([#211](https://github.com/NVIDIA/topograph/pull/211)).

### Changed

- **AWS SDK** upgraded for Secondary Networks support ([#214](https://github.com/NVIDIA/topograph/pull/214)).
- Kubernetes client libraries upgraded to **Kubernetes 1.34** ([#201](https://github.com/NVIDIA/topograph/pull/201)).
- HTTP helper functions simplified; accurate HTTP error propagation on topology requests ([#177](https://github.com/NVIDIA/topograph/pull/177), [#197](https://github.com/NVIDIA/topograph/pull/197)).
- Backend switch tier naming normalized ([#208](https://github.com/NVIDIA/topograph/pull/208)).
- **`model_path`** replaced with **`modelFileName`** in simulation config ([#212](https://github.com/NVIDIA/topograph/pull/212)).
- Nebius provider enhanced with Go SDK integration ([#227](https://github.com/NVIDIA/topograph/pull/227)).
- Helm subchart defaults simplified ([#238](https://github.com/NVIDIA/topograph/pull/238)).

### Fixed

- Node observer triggers topology discovery when **watched pods become Ready** ([#183](https://github.com/NVIDIA/topograph/pull/183)).
- **`/v1/topology`** returns **HTTP 202 Accepted** while a request is still in progress ([#192](https://github.com/NVIDIA/topograph/pull/192), [#193](https://github.com/NVIDIA/topograph/pull/193), [#210](https://github.com/NVIDIA/topograph/pull/210)).
- HTTP client option to **skip TLS verification** for lab environments ([#181](https://github.com/NVIDIA/topograph/pull/181)).
- Slinky login-pod discovery requires **Running** state ([#188](https://github.com/NVIDIA/topograph/pull/188)).
- Slurm block ordering ([#207](https://github.com/NVIDIA/topograph/pull/207)).
- GCP project ID resolution from credentials ([#220](https://github.com/NVIDIA/topograph/pull/220)).
- DRA provider rejects empty domain sets ([#189](https://github.com/NVIDIA/topograph/pull/189)).
- Partition topology omits nodes with missing data ([#229](https://github.com/NVIDIA/topograph/pull/229)).
- Kubernetes resource requests and limits in the Helm chart ([#231](https://github.com/NVIDIA/topograph/pull/231)).
- NVIDIA device plugin DaemonSet name and namespace configurable for node-data-broker ([#237](https://github.com/NVIDIA/topograph/pull/237)).
- Retry logic and HTTP error logging in the API server ([#196](https://github.com/NVIDIA/topograph/pull/196), [#209](https://github.com/NVIDIA/topograph/pull/209)).
- Request latency histogram buckets tuned for percentile accuracy ([#215](https://github.com/NVIDIA/topograph/pull/215)).

[Full changelog](https://github.com/NVIDIA/topograph/compare/v0.1.0...v0.3.0)

---

## [0.1.0] - 2025-10-30

Initial release.

### Added

- Core **Topograph API server** with `/v1/generate` and asynchronous topology retrieval.
- **Providers**: AWS, GCP, OCI, Nebius, InfiniBand (bare-metal and Kubernetes), DRA, and CoreWeave (`cw`).
- **Engines**: Kubernetes (node labels), Slurm (`topology.conf`), and Slinky (ConfigMap).
- **Kubernetes components**: node-observer Deployment and node-data-broker DaemonSet (Helm subcharts).
- **Helm chart** for deploying Topograph on Kubernetes.
- Provider and engine documentation under `docs/providers/` and `docs/engines/`.
- Container images published to `ghcr.io/nvidia/topograph`.
- Debian and RPM packaging targets.

[Release notes](https://github.com/NVIDIA/topograph/releases/tag/v0.1.0)

---

[Unreleased]: https://github.com/NVIDIA/topograph/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/NVIDIA/topograph/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/NVIDIA/topograph/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/NVIDIA/topograph/compare/v0.1.0...v0.3.0
[0.1.0]: https://github.com/NVIDIA/topograph/releases/tag/v0.1.0
