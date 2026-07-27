#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

# Demonstrates publishing simulated topology through Node Feature Discovery.

set -e

demo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

cd "$demo_dir/../.."

source demos/utils.sh

step "make build TARGETS=kwok-nodes"

step "delete_cluster"

step "./scripts/create-kind-kwok-cluster.sh -m ./tests/models/medium.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" get node"

step "demos/test-nfd/deploy-nfd.sh"

step "KUBE_CONTEXT=\"$KUBE_CONTEXT\" ./scripts/install-topograph.sh demos/test-nfd/values.nfd.kwok.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" -n node-feature-discovery get nodefeaturegroups"

step "kubectl --context \"$KUBE_CONTEXT\" -n node-feature-discovery get nodefeaturegroups -l topograph.nvidia.com/group-type=fabric-tier-1 -o yaml"
