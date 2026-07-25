#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

KUBE_CONTEXT="${KUBE_CONTEXT:-kind-topograph}"
SLINKY_VERSION="${SLINKY_VERSION:-1.2.0}"

CONTROL_PLANE_LABEL="node-role.kubernetes.io/control-plane"
KWOK_LABEL="kwok.x-k8s.io/node"

control_plane_selector='{"node-role.kubernetes.io/control-plane":""}'
control_plane_tolerations='[{"key":"node-role.kubernetes.io/control-plane","operator":"Exists","effect":"NoSchedule"}]'
kwok_selector='{"kwok.x-k8s.io/node":"fake"}'

control_plane_count="$(kubectl --context "$KUBE_CONTEXT" get nodes \
    -l "$CONTROL_PLANE_LABEL" -o name | wc -l | tr -d '[:space:]')"
if [[ "$control_plane_count" != "1" ]]; then
    echo "expected exactly one control-plane node, found $control_plane_count" >&2
    exit 1
fi

kwok_node_count="$(kubectl --context "$KUBE_CONTEXT" get nodes \
    -l "$KWOK_LABEL=fake" -o name | wc -l | tr -d '[:space:]')"
if [[ "$kwok_node_count" == "0" ]]; then
    echo "expected at least one KWOK node with $KWOK_LABEL=fake" >&2
    exit 1
fi

helm upgrade --install slurm-operator \
    oci://ghcr.io/slinkyproject/charts/slurm-operator \
    --version "$SLINKY_VERSION" \
    --kube-context "$KUBE_CONTEXT" \
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
    --version "$SLINKY_VERSION" \
    --kube-context "$KUBE_CONTEXT" \
    --namespace slurm --create-namespace \
    --set controller.persistence.enabled=false \
    --set-json "controller.podSpec.nodeSelector=$control_plane_selector" \
    --set-json "controller.podSpec.tolerations=$control_plane_tolerations" \
    --set-json "restapi.podSpec.nodeSelector=$control_plane_selector" \
    --set-json "restapi.podSpec.tolerations=$control_plane_tolerations" \
    --set nodesets.kwok.scalingMode=DaemonSet \
    --set-json "nodesets.kwok.podSpec.nodeSelector=$kwok_selector" \
    --set partitions.all.enabled=true \
    --set-string 'configFiles.topology\.conf=' \
    --wait --timeout 10m

# KWOK marks pods Ready without running their containers. Keep the simulated
# slurmd pods from being deleted for failing to register with Slurm.
kubectl --context "$KUBE_CONTEXT" patch nodeset slurm-worker-kwok \
    --namespace slurm \
    --type merge \
    --patch '{"spec":{"minReadySeconds":2147483647}}'

min_ready_seconds="$(kubectl --context "$KUBE_CONTEXT" get nodeset slurm-worker-kwok \
    --namespace slurm \
    -o jsonpath='{.spec.minReadySeconds}')"
if [[ "$min_ready_seconds" != "2147483647" ]]; then
    echo "failed to configure simulated slurmd pod stability" >&2
    exit 1
fi

echo "Slinky $SLINKY_VERSION deployed to $KUBE_CONTEXT with $kwok_node_count KWOK compute nodes"
