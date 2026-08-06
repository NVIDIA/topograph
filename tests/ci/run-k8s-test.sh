#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if (( $# != 2 )); then
    echo "usage: $0 <values-file> <expected-accelerator>" >&2
    exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"
values_file="$1"
expected_accelerator="$2"
kube_context="${KUBE_CONTEXT:-kind-topograph}"
test_node="${TEST_NODE:-1202}"
wait_attempts="${WAIT_ATTEMPTS:-60}"

if [[ "$values_file" != /* ]]; then
    values_file="$repo_root/$values_file"
fi

"$repo_root/scripts/install-topograph.sh" "$values_file"

for attempt in $(seq 1 "$wait_attempts"); do
    if kubectl --context "$kube_context" get node "$test_node" -o json |
        jq -e --arg accelerator "$expected_accelerator" '
            .metadata.labels["network.topology.nvidia.com/tier-0"] == "sw12" and
            .metadata.labels["network.topology.nvidia.com/tier-1"] == "sw21" and
            .metadata.labels["network.topology.nvidia.com/tier-2"] == "sw3" and
            .metadata.labels["xclr.topology.nvidia.com/domain"] == $accelerator
        ' >/dev/null; then
        exit 0
    fi
    sleep 2
done

echo "::error::Timed out waiting for Topograph labels on node $test_node"
kubectl --context "$kube_context" get node "$test_node" -o yaml
exit 1
