# Topograph Chainsaw E2E Tests

End-to-end conformance tests for the Kubernetes and Slinky engines using
[Chainsaw](https://kyverno.github.io/chainsaw/) — Kyverno's declarative
`apply → wait → assert → cleanup` framework.

## How the tests work

All suites follow the same high-level cycle: prepare cluster state → install
Topograph → assert outputs → clean up. There are three preparation patterns.

### Pattern A — Test provider with fake nodes

Used by: `k8s/label-application`, `k8s/label-truncation`, `slinky/block-complement`,
`slinky/dynamic-nodes`

1. Creates a `topology-test-model` ConfigMap with the topology model embedded inline,
   mounted at `/etc/topograph/models/` in the Topograph pod.
2. Creates fake K8s Node objects carrying `kubernetes.io/os=linux` so the Node Observer
   fires on them. k8s-engine suites also carry `topograph.nvidia.com/instance` and
   `topograph.nvidia.com/region` annotations so the engine can map instance IDs to nodes.
3. Slinky block-topology suites additionally create one fake slurmd pod per fake node
   (status-patched to `Ready`) so the Slinky engine can build its
   k8s-node→SLURM-hostname map for writing the ConfigMap and annotating nodes.
4. Installs the Topograph Helm chart with the Node Observer enabled. The observer
   fires on the fake nodes on startup, auto-triggering `/v1/generate` — no manual
   HTTP POST needed.
5. Asserts that the expected node labels (k8s engine) or ConfigMap content (Slinky
   engine) appear.
6. Cleans up (uninstalls the chart, deletes the fake nodes and namespace).

### Pattern B — DRA provider with fake nodes

Used by: `slinky/dra-provider`

1. Creates fake K8s nodes carrying `nvidia.com/gpu.clique`,
   `topograph.nvidia.com/instance`, and `topograph.nvidia.com/region`
   labels/annotations. The DRA provider reads NVLink clique topology directly from
   the K8s API — no model ConfigMap is needed.
2. Creates one fake slurmd pod per fake node (status-patched to `Ready`) so the
   Slinky engine can build its k8s-node→SLURM-hostname map.
3. Installs the Helm chart with the Node Observer enabled; the observer fires on the
   fake nodes and auto-triggers generation.
4. Asserts the `slurm-topology` ConfigMap contains the correct block topology entries.
5. Cleans up.

### Pattern C — Test provider with real cluster nodes

Used by: `slinky/tree-topology`

1. Creates a `topology-test-model` ConfigMap with the model embedded inline.
2. Installs the Helm chart with the Node Observer watching all `kubernetes.io/os=linux`
   nodes. Kind worker nodes already carry this label, so generation is triggered
   immediately on startup — no fake nodes are created.
3. Topology entries are derived entirely from the model's switch structure; no slurmd
   pods are needed.
4. Asserts the `slurm-topology` ConfigMap contains the correct tree-format entries.
5. Cleans up.

## Test suites

| Suite | Topology source | What it checks |
|---|---|---|
| `k8s/label-application` | Test provider — inline model `s1→{s2,s3}`, nodes `node-01` (under s2) and `node-02` (under s3); two fake K8s nodes | `leaf`, `spine` labels applied correctly to fake nodes |
| `k8s/label-truncation` | Test provider — inline model `s1→AVERYLONGSWITCHNAMETHATEXCEEDSSIXTYCHARACTERSFORTESTINGPURPOSES01→node-01`; one fake K8s node | Switch names >63 chars are replaced with an FNV64a hash prefixed with `x` |
| `slinky/tree-topology` | Test provider — inline model `S1→{S2,S3}`, nodes `node-01` and `node-02`; fires on real kind worker nodes (no fake nodes) | Slinky engine writes correct `topology.conf` (tree format) into a ConfigMap |
| `slinky/dra-provider` | DRA provider — `nvidia.com/gpu.clique` labels on four fake nodes (clique-1: node-01/node-02, clique-2: node-03/node-04); fake slurmd pods | DRA provider discovers NVLink clique topology from node labels; Slinky engine writes correct `topology.conf` (block topology) into a ConfigMap |
| `slinky/block-complement` | Test provider — inline model: spine→{leaf-1,leaf-2,leaf-3}, three NVLink cliques with node-02 (clique-1) and node-05 (clique-3) absent; four fake K8s nodes and fake slurmd pods | Slinky engine pads the block tree with an empty `BlockName=block004` placeholder when BlockSizes=2,4,8 and only 3 of 4 base-block slots are filled; absent nodes are not emitted in their BlockName line |
| `slinky/dynamic-nodes` | Test provider — same three-clique model as `block-complement` (node-02/05 absent); four fake K8s nodes and fake slurmd pods; `useDynamicNodes: true`, `configUpdateMode: skeleton-only` | Slinky engine writes all `BlockName` lines without `Nodes=` (skeleton format) and `performReconciliation` annotates each K8s node with `topology.slinky.slurm.net/spec` pointing to its assigned block |

## Prerequisites

| Tool | Install |
|---|---|
| `chainsaw` | `brew install kyverno/tap/chainsaw` or see [docs](https://kyverno.github.io/chainsaw/latest/quick-start/install/) |
| `kind` | `brew install kind` |
| `helm` | `brew install helm` |
| `kubectl` | `brew install kubectl` |
| `docker` | [Docker Desktop](https://www.docker.com/products/docker-desktop/) |

## Quick start — local kind cluster

```bash
# Build image, create cluster, run all suites, delete cluster
make e2e-local
```

`make e2e-local` runs in sequence:
1. `make image-build` — builds the container image for `linux/<host-arch>`
2. `kind create cluster` — spins up a 4-worker kind cluster (`tests/chainsaw/kind-config.yaml`)
3. `kind load docker-image` — loads the local image into the cluster with `imagePullPolicy: Never`
4. `chainsaw test` — runs all suites
5. `kind delete cluster` — tears down the cluster

## Running against an existing kind cluster

If you already have a kind cluster and want to run the tests without tearing it
down, the three-step sequence is:

```bash
make image-build                             # 1. build the image (tagged with the current commit SHA)
make kind-load KIND_CLUSTER=<cluster-name>   # 2. load that image into the cluster
make e2e                                     # 3. run all suites
```

`IMAGE_TAG` defaults to `$(git rev-parse --short HEAD)`. Because it is tied to
the commit SHA, you must rebuild and reload whenever you commit new changes —
otherwise the cluster has a stale image or the tag does not exist at all.

To use a fixed tag instead of the SHA:

```bash
make image-build E2E_IMAGE_TAG=my-tag
make kind-load KIND_CLUSTER=<cluster-name> E2E_IMAGE_TAG=my-tag
make e2e E2E_IMAGE_TAG=my-tag
```

## Running against a non-kind cluster

For a cluster where the image is already in a reachable registry, pass the
repo and tag as Make variable overrides (not shell env vars — the Makefile
uses `IMAGE_REPO` and `E2E_IMAGE_TAG`, not `TOPOGRAPH_IMAGE_REPO`/`TOPOGRAPH_IMAGE_TAG`):

```bash
make e2e IMAGE_REPO=my-registry/topograph E2E_IMAGE_TAG=my-tag
```

## Running a single suite

```bash
chainsaw test --test-dir tests/chainsaw/k8s/label-application
```

To pass a specific image:

```bash
TOPOGRAPH_IMAGE_REPO=ghcr.io/nvidia/topograph \
TOPOGRAPH_IMAGE_TAG=my-tag \
chainsaw test --test-dir tests/chainsaw/k8s/label-application
```

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `TOPOGRAPH_IMAGE_REPO` | `ghcr.io/nvidia/topograph` | Image repository |
| `TOPOGRAPH_IMAGE_TAG` | `` (chart `appVersion`) | Image tag passed directly to test scripts |
| `E2E_IMAGE_TAG` | short commit SHA (`git rev-parse --short HEAD`) | Tag used by `make e2e` / `make e2e-local` / `make kind-load` |
| `TOPOGRAPH_IMAGE_PULL_POLICY` | `IfNotPresent` | Set to `Never` for kind (done automatically by `make e2e-local`) |
