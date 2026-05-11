# Topograph Overview

## What Topograph Is

Topograph is a component that **discovers the physical network topology of a cluster** and **exposes it to workload schedulers** (Slurm, Kubernetes, and Slurm-on-Kubernetes / Slinky) in the format each one expects.


## The Problem

At scale, workload placement becomes as important as resource allocation. Where a job runs can have a significant impact on its performance. For example, a distributed training job that spans nodes on opposite sides of a data center fabric incurs additional latency during every gradient synchronization. Similarly, a disaggregated inference pipeline that ignores NVLink locality may fail to utilize the full interconnect bandwidth available. In both cases, overlooking the underlying network topology leads to inefficient execution.

The physical network topology of a cluster plays a critical role in determining application performance. Modern GPU clusters are typically built on multi-tier network fabrics, where communication costs vary depending on node placement. Nodes connected to the same leaf switch can communicate with lower latency and higher bandwidth than nodes separated by multiple network hops. On advanced systems such as GB200/GB300 NVL72, groups of nodes share a high-speed NVLink fabric, forming a locality domain that significantly outperforms even the fastest Ethernet or InfiniBand connections.

Disaggregated inference serving illustrates the cost of ignoring topology. In these architectures, the prefill phase (processing the input prompt and producing the KV cache) and the decode phase (generating output tokens) run on separate, independently scalable GPU pools. At the boundary between them, the KV cache must transfer from prefill workers to decode workers — a large, latency-sensitive operation whose speed is entirely determined by the network path between the two pools. When prefill and decode workers are co-located within the same NVLink fabric the transfer is fast; when they land on opposite sides of a spine boundary and fall back to Ethernet, throughput drops and tail latency spikes.

To make optimal placement decisions, schedulers must be aware of this topology. However, topology information is often fragmented across multiple sources, including cloud provider APIs, fabric management systems, and low-level system tools. Each source exposes data through different interfaces and formats. At the same time, workload managers, such as Slurm, Kubernetes, or Slurm-on-Kubernetes deployments like Slinky, require this information in their own specific formats.

Topograph addresses this challenge. It discovers the physical network topology of a cluster and exposes it to schedulers in a form they can consume. By abstracting over diverse topology sources and translating them into the required output formats, Topograph transforms what would otherwise be a manual, environment-specific process into a unified and extensible pipeline.

## How It Works

Topograph operates around two primary concepts: a `provider` and an `engine`.

A provider represents a cloud or on-premises environment, while an engine refers to a scheduling system such as SLURM, Kubernetes, or hybrid SLURM-on-Kubernetes platforms such as Slinky.

This design allows Topograph to abstract across multiple topology sources — including CSP APIs, NVIDIA NetQ, and system-level tools — and translate that information into scheduler-specific outputs, such as Slurm topology.conf, Kubernetes node labels, or Slinky ConfigMaps.

Topograph runs as a service and exposes several endpoints, which are described in detail in API.md.

Topology discovery is performed asynchronously. The /v1/generate endpoint first receives and validates a topology request, then returns a request ID. That request ID can be used with the /v1/topology endpoint to check the request status and retrieve the resulting topology data once the operation has completed successfully.

## Supported Environments

Currently supported providers:

- [AWS](./providers/aws.md)
- [OCI](./providers/oci.md)
- [GCP](./providers/gcp.md)
- [Nebius](./providers/nebius.md)
- [Nscale](./providers/nscale.md)
- [NetQ](./providers/netq.md)
- [DRA](./providers/dra.md) — reads `nvidia.com/gpu.clique` labels set by the NVIDIA GPU operator DRA driver
- [InfiniBand (bare-metal)](./providers/infiniband.md#infiniband-bm-bare-metal)
- [InfiniBand (Kubernetes)](./providers/infiniband.md#infiniband-k8s-kubernetes)
- [Test](./providers/test.md) - simulates Topograph success, pending, and error responses for integration testing

Currently supported engines:

- [SLURM](./engines/slurm.md)
- [Kubernetes](./engines/k8s.md)
- [SLURM-on-Kubernetes (Slinky)](./engines/slinky.md)

### Choosing a Provider

| Scenario | Recommended provider |
|---|---|
| Cloud cluster (AWS, GCP, OCI, Nebius, Nscale) | Use the matching CSP provider |
| Spectrum-X fabric | [NetQ](./providers/netq.md) |
| Multi-Node NVLink (MNNVL), infrastructure visibility | [NetQ](./providers/netq.md) |
| MNNVL on Kubernetes (scheduling) | [DRA](./providers/dra.md) |
| InfiniBand fabric, NetQ deployed | [NetQ](./providers/netq.md) |
| InfiniBand fabric, no NetQ, bare-metal / Slurm | [InfiniBand (bare-metal)](./providers/infiniband.md) |
| InfiniBand fabric, no NetQ, Kubernetes | [InfiniBand (Kubernetes)](./providers/infiniband.md) |
| Client integration and regression testing | [Test](./providers/test.md) |

For MNNVL environments, NetQ and DRA operate at different layers and can coexist: NetQ provides infrastructure-level visibility into the NVLink fabric while DRA feeds topology directly to Kubernetes schedulers via `nvidia.com/gpu.clique` node labels.

For non-MNNVL GPU clusters (such as DGX B200 or B300 SuperPODs), `nvidia.com/gpu.clique` is not set — Topograph with an InfiniBand provider is the only source of network topology for scheduling decisions on these systems.

## Learn more

- [Architecture](./architecture.md)
- [Configuration and API](./api.md)
