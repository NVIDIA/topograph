#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# Demonstrates Kubernetes node labeling from a simulated Topograph model.

set -e

demo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$demo_dir/../.."

source demos/utils.sh

step "make build TARGETS=kwok-nodes"

step "delete_cluster"

step "./scripts/create-kind-kwok-cluster.sh -m ./tests/models/dual-accelerator.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" get node"

step "kubectl --context \"$KUBE_CONTEXT\" get node srv5103 -o yaml | yq '.metadata.labels'"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/install-topograph.sh demos/oci-sim-k8s/values.k8s.kwok.yaml"

sleep 5
step "kubectl --context \"$KUBE_CONTEXT\" get node srv5103 -o yaml | yq '.metadata.labels'"
