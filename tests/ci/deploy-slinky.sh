#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

kube_context="${KUBE_CONTEXT:-kind-topograph}"
slinky_version="${SLINKY_VERSION:-1.2.0}"

control_plane_selector='{"node-role.kubernetes.io/control-plane":""}'
control_plane_tolerations='[{"key":"node-role.kubernetes.io/control-plane","operator":"Exists","effect":"NoSchedule"}]'

control_plane_count="$(kubectl --context "$kube_context" get nodes \
    -l node-role.kubernetes.io/control-plane -o name | wc -l | tr -d '[:space:]')"
if [[ "$control_plane_count" != "1" ]]; then
    echo "expected exactly one control-plane node, found $control_plane_count" >&2
    exit 1
fi

helm upgrade --install slurm-operator \
    oci://ghcr.io/slinkyproject/charts/slurm-operator \
    --version "$slinky_version" \
    --kube-context "$kube_context" \
    --namespace slinky --create-namespace \
    --set crds.enabled=true \
    --set certManager.enabled=false \
    --set-json "operator.nodeSelector=$control_plane_selector" \
    --set-json "operator.tolerations=$control_plane_tolerations" \
    --set-json "webhook.nodeSelector=$control_plane_selector" \
    --set-json "webhook.tolerations=$control_plane_tolerations" \
    --wait --timeout 5m

helm upgrade --install slurm \
    oci://ghcr.io/slinkyproject/charts/slurm \
    --version "$slinky_version" \
    --kube-context "$kube_context" \
    --namespace slurm --create-namespace \
    --set controller.persistence.enabled=false \
    --set-json "controller.podSpec.nodeSelector=$control_plane_selector" \
    --set-json "controller.podSpec.tolerations=$control_plane_tolerations" \
    --set-json "restapi.podSpec.nodeSelector=$control_plane_selector" \
    --set-json "restapi.podSpec.tolerations=$control_plane_tolerations" \
    --set-string 'configFiles.topology\.conf=' \
    --wait --timeout 10m

echo "Slinky $slinky_version deployed to $kube_context without compute nodes"
