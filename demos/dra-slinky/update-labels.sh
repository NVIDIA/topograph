#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

KUBE_CONTEXT="${KUBE_CONTEXT:-kind-topograph}"

kubectl --context "$KUBE_CONTEXT" get nodes -l kwok.x-k8s.io/node=fake \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' |
while read -r node; do
    value=$(kubectl --context "$KUBE_CONTEXT" get node "$node" \
        -o jsonpath='{.metadata.labels.network\.topology\.nvidia\.com/accelerator}')
    if [ -n "$value" ]; then
        kubectl --context "$KUBE_CONTEXT" label node "$node" \
            "nvidia.com/gpu.clique=$value" "kubernetes.io/os=linux" --overwrite
        kubectl --context "$KUBE_CONTEXT" label node "$node" \
            network.topology.nvidia.com/accelerator-
    fi
done
