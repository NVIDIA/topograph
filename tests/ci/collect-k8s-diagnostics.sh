#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -u

kube_context="${KUBE_CONTEXT:-kind-topograph}"
topograph_namespace="${TOPOGRAPH_NAMESPACE:-topograph}"
topograph_release="${TOPOGRAPH_RELEASE:-topograph}"

run() {
    printf '\033[33m+'
    printf ' %q' "$@"
    printf '\033[0m\n'
    "$@" || true
}

run kubectl --context "$kube_context" get nodes --show-labels

run kubectl --context "$kube_context" get pods --all-namespaces -o wide

run kubectl --context "$kube_context" --namespace "$topograph_namespace" logs \
    --selector "app.kubernetes.io/instance=$topograph_release" \
    --all-containers --prefix --tail=-1
