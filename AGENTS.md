# AGENTS.md

This file provides guidance to Codex, Cursor, Copilot, and other coding agents when working with code in this repository.

<!-- AUTO-SYNCED: canonical source is .claude/CLAUDE.md. Only the first 5 lines differ. -->

## 1. Project Overview and Architecture

Topograph discovers the physical network topology of a cluster (NVLink domains, InfiniBand/Ethernet switch fabric, cloud rack topology) and exposes it to workload schedulers — Slurm, Kubernetes, and Slurm-on-Kubernetes (Slinky). It has five runtime components:

- **API Server** — receives `/v1/generate` requests, aggregates bursts, dispatches to a Provider
- **Node Observer** — Kubernetes-only; watches node status changes and triggers regeneration
- **Node Data Broker** — Kubernetes-only DaemonSet; collects per-node attributes (NVLink clique IDs, etc.) as node annotations
- **Provider** — per-environment adapter that queries a topology source (CSP API, NetQ, `ibnetdiscover`, DRA labels) and returns a canonical representation
- **Engine** — per-scheduler translator that writes the canonical representation out as `topology.conf`, Kubernetes node labels, or a Slinky ConfigMap

### Key invariant

Providers differ by environment. The canonical `*topology.Vertex` tree is stable. Engines only translate — they do not discover.

This separation is load-bearing. If you find yourself reading the fabric in an engine, or emitting scheduler-specific output from a provider, stop and reconsider.

### Repository map

```
cmd/                  # Four entry points: topograph, node-observer, node-data-broker-initc, toposim
pkg/
  providers/          # One directory per provider: aws, gcp, oci, nebius, netq, dra, infiniband, lambdai, cw, test
  engines/            # One directory per engine: k8s, slinky, slurm
  topology/           # Canonical Vertex tree and topology constants (DO NOT CHANGE CASUALLY)
  registry/           # Central NamedLoader wiring for providers + engines
  translate/          # topology.conf and block/tree generation shared by engines
  server/             # HTTP server and request aggregator
  node_observer/      # Kubernetes Node watcher
  ib/                 # InfiniBand fabric discovery helpers
  toposim/            # Simulation harness used by provider `*sim*` variants
  config/             # Config file parser
  protos/             # Generated protobuf (pb.go — do not hand-edit)
  metrics/            # Prometheus metrics
  models/             # Go types and loader for YAML simulation models (the YAML files live in tests/models/)
  test/               # Cross-package test helpers
internal/             # Shared utilities not part of the public API
  cluset, component, config, exec, files, httperr, httpreq, k8s, version
protos/               # .proto sources (regenerate with `make proto`)
charts/topograph/     # Helm chart (with node-data-broker subchart)
docs/                 # Public-facing docs — overview.md, architecture.md, api.md + providers/, engines/, reference/ subdirectories
tests/models/         # YAML simulation fixtures
config/               # Sample topograph-config.yaml
scripts/              # Build scripts (deb, rpm, SSL, clean)
localdev/             # Developer-local workspace — not tracked; personal scratch files
```

### Do not change without discussion

These structures propagate across every provider and engine. Changing them in a single PR usually means the PR is too broad.

| Surface | Why it's load-bearing |
|---|---|
| `pkg/topology/` — the `Vertex` tree and topology constants | Every provider returns it; every engine consumes it. A shape change ripples to all of them. |
| `protos/topology.proto` | Used by `forwardServiceUrl` gRPC forwarding. Breaking changes require regeneration (`make proto`) and coordinated client updates. |
| Helm `global.provider.name` / `global.engine.name` / `topologyNodeLabels` | External contract for operators deploying Topograph. |
| The four default label keys `network.topology.nvidia.com/{accelerator,leaf,spine,core}` | Consumed by downstream projects (KAI Scheduler, NVSentinel, Kueue). |

## 2. Setup and Installation

### Prerequisites

- **Go 1.24.7** (see `go.mod`) — newer minor versions are fine; older will not build
- **make**
- **golangci-lint** — `brew install golangci-lint` or via `go install`
- **docker** — only for container image builds and the IB variant
- **protoc** + plugins — only when regenerating protobuf (`make init-proto` installs the plugins)

### Clone and build

```bash
git clone https://github.com/NVIDIA/topograph.git
cd topograph
make build   # produces bin/topograph, bin/node-observer, bin/node-data-broker-initc
```

Cross-compile with `make build-linux-amd64`, `make build-darwin-arm64`, etc.

### Protobuf regeneration

Only when `protos/*.proto` changes:

```bash
make init-proto   # one-time: installs protoc-gen-go and protoc-gen-go-grpc
make proto        # regenerates pkg/protos/*.pb.go
```

Commit the regenerated files alongside the `.proto` change.

## 3. Testing and Deployment Workflows

### Local test loop

```bash
make qualify  # runs fmt, vet, lint, and test in sequence — pre-push aggregator
make fmt      # go fmt ./...
make vet      # go vet ./...
make lint     # golangci-lint run (only flags new issues vs. main)
make test     # go test -race -coverprofile=coverage.out ./...
make coverage # human-readable per-package summary
```

Run `make qualify` before pushing. The individual targets are available if you want to run a single check during iteration.

### Coverage policy

From `codecov.yml`:
- **Project coverage**: 60% target, 5% threshold for drops
- **Patch coverage**: 50% target, 5% threshold

Coverage checks run on pull requests. A drop below target with no matching uplift in the touched files will fail the Codecov check.

### CI workflows

- `.github/workflows/go.yml` — build, test, lint on every push and PR
- `.github/workflows/docker.yml` — container image build (manual trigger)
- `.github/workflows/docker-ib.yml` — InfiniBand-variant container (manual trigger)
- `.github/workflows/helm-release.yaml` — Helm chart release (manual trigger)

### Deployment surfaces

- **Binaries** — `deb` and `rpm` packages via `make deb` / `make rpm` (consumed by Slurm users)
- **Container images** — `ghcr.io/nvidia/topograph` (consumed by Kubernetes users)
- **Helm chart** — `charts/topograph/` (with `node-data-broker` subchart)

## 4. Coding Style and Conventions

### Formatting and linting

- `go fmt ./...` is authoritative — do not hand-format
- `golangci-lint` runs in CI with `--new-from-rev` so only new issues block; fix warnings in code you touch
- Copyright header on every new Go file: `Copyright (c) <year>, NVIDIA CORPORATION.  All rights reserved.` followed by the Apache 2.0 boilerplate matching existing files

### Provider interface

The contract lives in `pkg/providers/providers.go`:

```go
type Provider interface {
    GenerateTopologyConfig(
        ctx context.Context,
        pageSize *int,
        instances []topology.ComputeInstances,
    ) (*topology.Vertex, *httperr.Error)
}
```

A provider returns the root `*topology.Vertex` of the discovered tree. Leaf vertices are compute nodes; interior vertices are switches or (for block topology) accelerator domains. Return `*httperr.Error` so the API server can propagate the correct HTTP status code — plain `error` is not acceptable at this boundary.

### Adding a new provider

1. Create `pkg/providers/<name>/` with at minimum `provider.go` and `provider_test.go`
2. Expose a `NamedLoader` function with signature `func NamedLoader() (string, providers.Loader)` — this is how the registry wires the provider
3. Register in `pkg/registry/registry.go` by adding `<name>.NamedLoader` to the `providers.NewRegistry(...)` call list
4. Add `docs/providers/<name>.md` following the shape of `aws.md` / `netq.md` (prerequisites, credentials, parameters, how it works, verification)
5. Update `docs/overview.md` — add the provider to the "Currently supported providers" list and the "Choosing a Provider" scenario table
6. If the provider has a simulated variant for testing, export a second `NamedLoaderSim` and register it alongside (see `aws`, `gcp`, `oci`, `lambdai`)

### Adding a new engine

Engines are much rarer (three exist: slurm, k8s, slinky). Follow the same registry pattern but register in `engines.NewRegistry(...)`. Coordinate with maintainers before starting — adding an engine implies a new output format that every provider's output must be translatable into.

### Anti-patterns

| Don't | Because |
|---|---|
| Read the fabric inside an engine | Engines only translate; discovery belongs in providers |
| Emit scheduler-specific output from a provider | Same invariant in reverse |
| Change `pkg/topology/Vertex` fields without discussion | Every provider and engine depends on the shape |
| Hand-edit `pkg/protos/*.pb.go` | These are regenerated by `make proto`; edit `protos/*.proto` instead |
| Add a new provider in `pkg/providers/<name>/` without also updating `pkg/registry/registry.go` | Orphaned code; provider will not be loadable |
| Modify an AGENTS.md-described surface (new Makefile target, top-level directory, chart template, invariant) without updating `AGENTS.md` + `.claude/CLAUDE.md` in the same PR | Drift between the code and its agent-facing description; the next contributor / agent reads stale guidance |
| Skip DCO sign-off to "fix later" | The DCO bot will block the PR; rebase with `--signoff` is always available |
| Use plain `error` at the provider interface boundary | Must be `*httperr.Error` so the API server returns the correct HTTP status |

### Label and annotation reference

Label keys written by the Kubernetes and Slinky engines are documented in `docs/reference/node-labels.md`. Do not invent new keys in provider or engine code — values flow through the canonical tree; keys are configured via Helm `topologyNodeLabels`.

## 5. Pull Request Guidelines

### Branch naming

Use a prefix that matches the change type: `feat/`, `fix/`, `docs/`, `chore/`, `refactor/`, `test/`. Example: `docs/agents-md`, `feat/crusoe-provider`.

### Commit messages

Conventional Commits format:

```
type(scope): short description

optional body

Signed-off-by: Your Name <you@example.com>
```

Type must be one of: `feat`, `fix`, `docs`, `chore`, `refactor`, `style`, `perf`, `test`, `build`, `ci`.

### DCO sign-off is required

Every commit must carry a `Signed-off-by:` trailer. There is no `.github/dco.yml` exemption on this repo — NVIDIA org membership does not bypass the DCO bot here. Two ways to add it:

```bash
git commit -s -m "feat(provider/foo): add Foo provider"        # adds trailer
git commit -s -S -m "..."                                       # sign-off + GPG sign
```

If a PR arrives without sign-off, rebase the branch to add it:

```bash
git rebase --signoff upstream/main
git push --force-with-lease
```

### GPG signing is optional but recommended

Configure once:
```bash
git config --global user.signingkey <key-id>
git config --global commit.gpgsign true
```

Signed commits get a **Verified** badge on GitHub. The GPG public key must be uploaded to your GitHub account.

### Documentation Impact Evaluation

Every PR should be evaluated for documentation impact before pre-push qualification. The following changes imply specific doc updates in the same PR:

| Change | Docs update required |
|---|---|
| New / changed / removed provider | `docs/providers/<name>.md` + `docs/overview.md` provider list + "Choosing a Provider" scenario table |
| New / changed / removed engine | `docs/engines/<engine>.md` |
| New / changed chart values schema | `charts/topograph/values.yaml` comments, `NOTES.txt` output, and any docs that reference the values |
| New / changed API endpoint, request parameter, or response field | `docs/api.md` |
| New / changed config schema (`topograph-config.yaml` fields, defaults, validation) | `docs/api.md` |
| New invariant or "do not change without discussion" surface | `AGENTS.md` + `.claude/CLAUDE.md` in the same PR |
| New Makefile target, top-level directory, or repository-layout change described by the repository map | `AGENTS.md` + `.claude/CLAUDE.md` in the same PR |

If a change falls outside these categories, it still warrants a moment's review for collateral doc drift.

### Pre-push checklist

- [ ] `make qualify` passes (runs fmt, vet, lint, test)
- [ ] New or changed public behavior is covered by a test
- [ ] Documentation impact evaluated per the table above — applicable doc updates are included in this PR
- [ ] `pkg/topology/` or `protos/` changes were discussed in an issue first
- [ ] Every commit has a DCO sign-off

### Review expectations

- All CI checks must be green before merge (Go build/test/lint, Codecov, DCO)
- Reviewers look for: adherence to the provider/engine boundary, test coverage on new code paths, doc updates when contract changes
- Breaking changes to the config schema, label keys, or `Vertex` shape are rejected unless discussed in an issue first

### When in doubt

Read `docs/` before asking. Provider-specific questions usually have answers in `docs/providers/<name>.md`. Label semantics are in `docs/reference/node-labels.md`. The scenario-to-provider mapping is in the "Choosing a Provider" table in `docs/overview.md`. API endpoints and config schema live in `docs/api.md`.
