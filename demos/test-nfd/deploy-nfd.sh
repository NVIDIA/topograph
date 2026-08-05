#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

KUBE_CONTEXT="${KUBE_CONTEXT:-kind-topograph}"

CONTROL_PLANE_LABEL="node-role.kubernetes.io/control-plane"

control_plane_node=$(kubectl --context "$KUBE_CONTEXT" get nodes -l "$CONTROL_PLANE_LABEL" -o jsonpath='{.items[0].metadata.name}')

kubectl --context "$KUBE_CONTEXT" label node "$control_plane_node" nfd-enabled=true

helm repo add --force-update nfd https://kubernetes-sigs.github.io/node-feature-discovery/charts

helm repo update

helm upgrade --install nfd nfd/node-feature-discovery \
  --kube-context "$KUBE_CONTEXT" \
  --namespace node-feature-discovery \
  --create-namespace \
  --set-string worker.nodeSelector.nfd-enabled=true \
  --set featureGates.NodeFeatureGroupAPI=true \
  --wait
