#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# Demonstrates DRA topology discovery and Slinky configuration using KWOK nodes.

set -e

demo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$demo_dir/../.."

source demos/utils.sh

step "make build TARGETS=kwok-nodes"

step "delete_cluster"

step "./scripts/create-test-cluster.sh -m ./tests/models/large.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" get node"

step "./demos/dra-slinky/update-labels.sh"

step "kubectl --context \"$KUBE_CONTEXT\" get node srv4101 -o yaml | yq '.metadata.labels'"

step "./demos/dra-slinky/deploy-slinky.sh"

step "kubectl --context \"$KUBE_CONTEXT\" -n slurm get cm slurm-config-extra -o yaml"

resource_version="$(kubectl --context "$KUBE_CONTEXT" -n slurm \
    get cm slurm-config-extra -o jsonpath='{.metadata.resourceVersion}')"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/install-topograph.sh demos/dra-slinky/values.dra-slinky.kwok.yaml"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/wait-configmap-update.sh slurm slurm-config-extra \"$resource_version\""

# step "kubectl --context \"$KUBE_CONTEXT\" -n topograph logs -l app.kubernetes.io/name=topograph"

step "kubectl --context \"$KUBE_CONTEXT\" -n slurm get cm slurm-config-extra -o jsonpath='{.data.topology\.conf}' | grep -v '#'"
