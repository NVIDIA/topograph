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

step "./scripts/create-test-cluster.sh -m ./tests/models/medium.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" label node \"\$(kubectl --context \"$KUBE_CONTEXT\" get nodes -l node-role.kubernetes.io/control-plane -o jsonpath='{.items[0].metadata.name}')\" nfd-enabled=true"

step "demos/test-nfd/deploy-nfd.sh"

step "deploy_topograph demos/test-nfd/values.nfd.kwok.yaml"

step "kubectl --context \"$KUBE_CONTEXT\" -n node-feature-discovery get nodefeaturegroups"
