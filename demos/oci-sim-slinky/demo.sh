#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# Demonstrates OCI-sim fabric discovery and Slinky tree topology on kind.

set -e

demo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$demo_dir/../.."

source demos/utils.sh

step "delete_cluster"

step "kind create cluster --name \"${KUBE_CONTEXT#kind-}\" --wait 120s"

step "kubectl --context \"$KUBE_CONTEXT\" get node"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/deploy-slinky.sh --compute-mode none"

step "kubectl --context \"$KUBE_CONTEXT\" -n slurm get cm slurm-config-extra -o yaml"

resource_version="$(kubectl --context "$KUBE_CONTEXT" -n slurm \
    get cm slurm-config-extra -o jsonpath='{.metadata.resourceVersion}')"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/install-topograph.sh demos/oci-sim-slinky/values.oci-sim-slinky.yaml"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/wait-configmap-update.sh slurm slurm-config-extra \"$resource_version\""

step "kubectl --context \"$KUBE_CONTEXT\" -n slurm get cm slurm-config-extra -o jsonpath='{.data.topology\.conf}' | grep -v '#'"
