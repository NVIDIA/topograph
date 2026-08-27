<p align="center">
  <a href="https://github.com/NVIDIA/topograph" target="_blank">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/topograph-logo-color-dark.png" />
      <img src="docs/assets/topograph-logo-color.png" width="200" alt="Topograph logo" />
    </picture>
  </a>
</p>

# Topograph

[![Go CI](https://img.shields.io/github/actions/workflow/status/NVIDIA/topograph/go.yml?branch=main&label=go%20ci&style=flat-square&labelColor=172033&logo=github&logoColor=white)](https://github.com/NVIDIA/topograph/actions/workflows/go.yml)
[![Coverage](https://img.shields.io/codecov/c/github/NVIDIA/topograph/main?label=coverage&style=flat-square&labelColor=172033&logo=codecov&logoColor=white)](https://codecov.io/gh/NVIDIA/topograph)
[![Chart Tests](https://img.shields.io/github/actions/workflow/status/NVIDIA/topograph/chart-test.yaml?label=chart%20tests&style=flat-square&labelColor=172033&logo=helm&logoColor=white)](https://github.com/NVIDIA/topograph/actions/workflows/chart-test.yaml)
[![K8s Tests](https://img.shields.io/github/actions/workflow/status/NVIDIA/topograph/k8s-test.yaml?label=k8s%20tests&style=flat-square&labelColor=172033&logo=kubernetes&logoColor=white)](https://github.com/NVIDIA/topograph/actions/workflows/k8s-test.yaml)
[![Release](https://img.shields.io/github/v/release/NVIDIA/topograph?label=release&style=flat-square&labelColor=172033&color=2563EB&logo=github&logoColor=white)](https://github.com/NVIDIA/topograph/releases)
[![License](https://img.shields.io/github/license/NVIDIA/topograph?label=license&style=flat-square&labelColor=172033&color=0891B2&logo=apache&logoColor=white)](https://github.com/NVIDIA/topograph/blob/main/LICENSE)
[![Docs](https://img.shields.io/badge/docs-latest-blue?style=flat-square&labelColor=172033&logo=nvidia&logoColor=white)](https://docs.nvidia.com/topograph)

Topograph is a component that discovers the physical network topology of a cluster and exposes it to schedulers, enabling topology-aware scheduling decisions. It abstracts multiple topology sources and translates them into the format required by each scheduler.

## Quick Start

Pick the install path that matches your scheduler:

- **Kubernetes** — the same Helm chart covers native Kubernetes scheduling (`k8s` engine), Node Feature Discovery CR output (`nfd` engine), and [Slinky](https://github.com/SlinkyProject) (Slurm-on-Kubernetes, `slinky` engine). See [Install on Kubernetes](docs/get-started/quickstart-k8s.md).
- **Slurm (bare metal)** — install a `.deb` or `.rpm` package on the Slurm head node and run Topograph as a systemd service. See [Install on Slurm](docs/get-started/quickstart-slurm.md).

## Learn more

- [Overview](docs/overview.md)
- [Architecture](docs/architecture.md)
- [Configuration and API](docs/api.md)
