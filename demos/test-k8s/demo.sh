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

step "./scripts/create-test-cluster.sh -m ./tests/models/medium.yaml"

step "deploy_topograph demos/test-k8s/values.k8s.kwok.yaml"

sleep 5
step "kubectl --context \"$KUBE_CONTEXT\" describe no 1301 | head -15"
