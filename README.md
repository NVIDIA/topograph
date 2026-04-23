<p align="center"><a href="https://github.com/NVIDIA/topograph" target="_blank"><img src="docs/assets/topograph-logo.png" width="100" alt="Logo"></a></p>

# Topograph

![Build Status](https://github.com/NVIDIA/topograph/actions/workflows/go.yml/badge.svg)
![Codecov](https://codecov.io/gh/NVIDIA/topograph/branch/main/graph/badge.svg)
![Static Badge](https://img.shields.io/badge/license-Apache_2.0-green)

Topograph is a component that discovers the physical network topology of a cluster and exposes it to schedulers, enabling topology-aware scheduling decisions. It abstracts multiple topology sources and translates them into the format required by each scheduler.

## Quick Start

Pick the install path that matches your scheduler:

- **Kubernetes** — the same Helm chart covers native Kubernetes scheduling (`k8s` engine) and [Slinky](https://github.com/SlinkyProject) (Slurm-on-Kubernetes, `slinky` engine). See [Install on Kubernetes](docs/get-started/quickstart-k8s.md).
- **Slurm (bare metal)** — install a `.deb` or `.rpm` package on the Slurm head node and run Topograph as a systemd service. See [Install on Slurm](docs/get-started/quickstart-slurm.md).

## Learn more

- [Overview](docs/overview.md)
- [Architecture](docs/architecture.md)
- [Configuration and API](docs/api.md)
