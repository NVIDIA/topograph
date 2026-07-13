#!/usr/bin/env bash
# Copyright (c) 2026, NVIDIA CORPORATION. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-topograph-nvml-mock}"
TOPOGRAPH_IMAGE="${TOPOGRAPH_IMAGE:-topograph}"
TOPOGRAPH_TAG="${TOPOGRAPH_TAG:-e2e-nvml-mock}"
RUNTIME_IMAGE="${RUNTIME_IMAGE:-debian:trixie-slim}"
TOPOGRAPH_RELEASE="${TOPOGRAPH_RELEASE:-topograph}"
TOPOGRAPH_NAMESPACE="${TOPOGRAPH_NAMESPACE:-topograph}"
NVML_MOCK_RELEASE="${NVML_MOCK_RELEASE:-nvml-mock}"
NVML_MOCK_NAMESPACE="${NVML_MOCK_NAMESPACE:-nvml-mock-system}"
SLINKY_OPERATOR_NAMESPACE="slinky"
SLURM_RELEASE="slurm"
SLURM_NAMESPACE="slurm"
SLURM_WORKER_SELECTOR="app.kubernetes.io/component=worker"
SLURM_CONTROLLER_POD="${SLURM_RELEASE}-controller-0"
SLURM_TOPOLOGY_CONFIGMAP="slurm-config-extra"
SLURM_TOPOLOGY_CONFIG_KEY="topology.conf"
SLINKY_TOPOLOGY_ANNOTATION="topology.slinky.slurm.net/spec"
PLATFORM="${PLATFORM:-linux/$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')}"
REUSE_CLUSTER=false
SKIP_BUILD=false

usage() {
    cat <<EOF
Usage: $0 [--reuse-cluster] [--skip-build]

Creates a kind cluster, builds and loads the local Topograph image, installs
nvml-mock and Slinky, then deploys the local Topograph Helm chart.

Environment overrides:
  CLUSTER_NAME          kind cluster name (default: ${CLUSTER_NAME})
  TOPOGRAPH_IMAGE       local image repository (default: ${TOPOGRAPH_IMAGE})
  TOPOGRAPH_TAG         local image tag (default: ${TOPOGRAPH_TAG})
  RUNTIME_IMAGE         Dockerfile runtime base (default: ${RUNTIME_IMAGE})
  TOPOGRAPH_RELEASE     Helm release name (default: ${TOPOGRAPH_RELEASE})
  TOPOGRAPH_NAMESPACE   Helm namespace (default: ${TOPOGRAPH_NAMESPACE})
  NVML_MOCK_RELEASE     nvml-mock Helm release name (default: ${NVML_MOCK_RELEASE})
  NVML_MOCK_NAMESPACE   nvml-mock Helm namespace (default: ${NVML_MOCK_NAMESPACE})
  KUBE_CONTEXT          kubectl/helm context (default: kind-\${CLUSTER_NAME})
  PLATFORM              docker build platform (default: ${PLATFORM})
EOF
}

log() {
    printf '==> %s\n' "$*"
}

require_tool() {
    if ! command -v "$1" >/dev/null 2>&1; then
        printf 'error: required tool %q not found in PATH\n' "$1" >&2
        exit 1
    fi
}

chart_fullname() {
    local release="$1"
    local chart="$2"

    if [[ "$release" == *"$chart"* ]]; then
        printf '%s' "$release"
    else
        printf '%s-%s' "$release" "$chart"
    fi
}

wait_for_pods() {
    local namespace="$1"
    local selector="$2"
    local description="$3"
    local timeout_seconds="${4:-300}"
    local deadline=$((SECONDS + timeout_seconds))

    while (( SECONDS < deadline )); do
        if [[ "$(kubectl --context "$KUBE_CONTEXT" -n "$namespace" get pods -l "$selector" --no-headers 2>/dev/null | wc -l | tr -d ' ')" -gt 0 ]]; then
            kubectl --context "$KUBE_CONTEXT" -n "$namespace" wait \
                --for=condition=ready pod \
                -l "$selector" \
                --timeout="${timeout_seconds}s"
            return
        fi
        sleep 5
    done

    printf 'error: timed out waiting for %s pods matching %q in namespace %q\n' "$description" "$selector" "$namespace" >&2
    kubectl --context "$KUBE_CONTEXT" -n "$namespace" get pods --show-labels || true
    exit 1
}

wait_for_topology_configmap() {
    local timeout_seconds="${1:-180}"
    local deadline=$((SECONDS + timeout_seconds))
    local managed_by=""
    local plugin=""
    local topology_conf=""

    while (( SECONDS < deadline )); do
        managed_by="$(kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get configmap "$SLURM_TOPOLOGY_CONFIGMAP" \
            -o go-template='{{ index .metadata.annotations "topograph.nvidia.com/topology-managed-by" }}' 2>/dev/null || true)"
        plugin="$(kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get configmap "$SLURM_TOPOLOGY_CONFIGMAP" \
            -o go-template='{{ index .metadata.annotations "topograph.nvidia.com/plugin" }}' 2>/dev/null || true)"
        topology_conf="$(kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get configmap "$SLURM_TOPOLOGY_CONFIGMAP" \
            -o go-template='{{ index .data "topology.conf" }}' 2>/dev/null || true)"

        if [[ "$managed_by" == "topograph" && "$plugin" == "topology/block" && "$topology_conf" == *"BlockName="* && "$topology_conf" == *"BlockSizes="* ]]; then
            return
        fi
        sleep 5
    done

    printf 'error: timed out waiting for Topograph-managed %s/%s ConfigMap key %q\n' "$SLURM_NAMESPACE" "$SLURM_TOPOLOGY_CONFIGMAP" "$SLURM_TOPOLOGY_CONFIG_KEY" >&2
    kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get configmap "$SLURM_TOPOLOGY_CONFIGMAP" -o yaml || true
    exit 1
}

wait_for_node_topology_annotations() {
    local timeout_seconds="${1:-180}"
    local deadline=$((SECONDS + timeout_seconds))
    local annotations=""
    local total=0
    local missing=false
    local node=""
    local spec=""

    while (( SECONDS < deadline )); do
        annotations="$(kubectl --context "$KUBE_CONTEXT" get nodes -l topograph.nvidia.com/e2e-worker=true \
            -o go-template='{{ range .items }}{{ .metadata.name }}{{ "\t" }}{{ index .metadata.annotations "topology.slinky.slurm.net/spec" }}{{ "\n" }}{{ end }}' 2>/dev/null || true)"
        total=0
        missing=false

        while IFS=$'\t' read -r node spec; do
            [[ -z "$node" ]] && continue
            total=$((total + 1))
            if [[ -z "$spec" ]]; then
                missing=true
            fi
        done <<< "$annotations"

        if (( total > 0 )) && [[ "$missing" == "false" ]]; then
            printf '%s\n' "$annotations"
            return
        fi
        sleep 5
    done

    printf 'error: timed out waiting for %q annotations on e2e worker nodes\n' "$SLINKY_TOPOLOGY_ANNOTATION" >&2
    kubectl --context "$KUBE_CONTEXT" get nodes -l topograph.nvidia.com/e2e-worker=true \
        -o go-template='{{ range .items }}{{ .metadata.name }}{{ "\t" }}{{ index .metadata.annotations "topology.slinky.slurm.net/spec" }}{{ "\n" }}{{ end }}' || true
    exit 1
}

wait_for_slurm_topology_file() {
    local timeout_seconds="${1:-180}"
    local deadline=$((SECONDS + timeout_seconds))
    local topology_conf=""

    while (( SECONDS < deadline )); do
        topology_conf="$(kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" exec "$SLURM_CONTROLLER_POD" -- \
            sh -c 'test -s /etc/slurm/topology.conf && cat /etc/slurm/topology.conf' 2>/dev/null || true)"

        if [[ "$topology_conf" == *"BlockName="* && "$topology_conf" == *"BlockSizes="* ]]; then
            printf '%s\n' "$topology_conf"
            return
        fi
        sleep 5
    done

    printf 'error: timed out waiting for /etc/slurm/topology.conf in %s/%s\n' "$SLURM_NAMESPACE" "$SLURM_CONTROLLER_POD" >&2
    kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" exec "$SLURM_CONTROLLER_POD" -- \
        sh -c 'ls -l /etc/slurm/topology.conf 2>/dev/null || true; cat /etc/slurm/topology.conf 2>/dev/null || true' >&2 || true
    exit 1
}

wait_for_slurm_topology() {
    local timeout_seconds="${1:-180}"
    local deadline=$((SECONDS + timeout_seconds))
    local output=""
    local topology_summary=""

    while (( SECONDS < deadline )); do
        output="$(kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" exec "$SLURM_CONTROLLER_POD" -- \
            scontrol show nodes 2>/dev/null || true)"
        topology_summary="$(awk '
            /^NodeName=/ {
                if (node != "" && topology != "") {
                    print node "\t" topology
                }
                node = $1
                sub(/^NodeName=/, "", node)
                topology = ""
            }
            /Topology=/ {
                for (i = 1; i <= NF; i++) {
                    if ($i ~ /^Topology=/) {
                        topology = $i
                        sub(/^Topology=/, "", topology)
                    }
                }
            }
            END {
                if (node != "" && topology != "") {
                    print node "\t" topology
                }
            }
        ' <<< "$output" | sort -k1,1)"

        if [[ -n "$topology_summary" ]]; then
            printf '%s\n' "$topology_summary"
            return
        fi
        sleep 5
    done

    printf 'error: timed out waiting for Slurm nodes to report loaded topology via scontrol\n' >&2
    if [[ -n "$output" ]]; then
        printf '%s\n' "$output" >&2
    fi
    kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get pods -l app.kubernetes.io/component=controller || true
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            usage
            exit 0
            ;;
        --reuse-cluster)
            REUSE_CLUSTER=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        *)
            printf 'error: unknown argument %q\n' "$1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FULL_TOPOGRAPH_IMAGE="${TOPOGRAPH_IMAGE}:${TOPOGRAPH_TAG}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"
TOPOGRAPH_FULLNAME="$(chart_fullname "$TOPOGRAPH_RELEASE" topograph)"
NODE_DATA_BROKER_FULLNAME="$(chart_fullname "$TOPOGRAPH_RELEASE" node-data-broker)"
TOPOGRAPH_VALUES_FILE="${REPO_ROOT}/tests/e2e/topograph-values.yaml"
KIND_CONFIG_FILE="${REPO_ROOT}/tests/e2e/kind-cluster.yaml"
NVML_MOCK_VALUES_FILE="${REPO_ROOT}/tests/e2e/nvml-mock-values.yaml"
SLINKY_VALUES_FILE="${REPO_ROOT}/tests/e2e/slinky-values.yaml"

require_tool docker
require_tool kind
require_tool kubectl
require_tool helm

cd "$REPO_ROOT"

CLUSTER_EXISTS=false
while IFS= read -r cluster; do
    if [[ "$cluster" == "$CLUSTER_NAME" ]]; then
        CLUSTER_EXISTS=true
        break
    fi
done < <(kind get clusters)

if [[ "$CLUSTER_EXISTS" == "true" ]]; then
    if [[ "$REUSE_CLUSTER" == "true" ]]; then
        log "reusing existing kind cluster ${CLUSTER_NAME}"
    else
        printf 'error: kind cluster %q already exists; pass --reuse-cluster to use it\n' "$CLUSTER_NAME" >&2
        exit 1
    fi
else
    log "creating kind cluster ${CLUSTER_NAME}"
    kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG_FILE"
fi

if [[ "$SKIP_BUILD" == "true" ]]; then
    log "skipping Topograph image build"
else
    log "building ${FULL_TOPOGRAPH_IMAGE} with RUNTIME_IMAGE=${RUNTIME_IMAGE}"
    docker buildx build \
        --platform "$PLATFORM" \
        --build-arg "RUNTIME_IMAGE=${RUNTIME_IMAGE}" \
        -t "$FULL_TOPOGRAPH_IMAGE" \
        --load .
fi

log "loading ${FULL_TOPOGRAPH_IMAGE} into kind cluster ${CLUSTER_NAME}"
kind load docker-image "$FULL_TOPOGRAPH_IMAGE" --name "$CLUSTER_NAME"

log "installing nvml-mock"
helm upgrade --install "$NVML_MOCK_RELEASE" \
    oci://ghcr.io/nvidia/k8s-test-infra/chart/nvml-mock \
    --kube-context "$KUBE_CONTEXT" \
    --namespace "$NVML_MOCK_NAMESPACE" \
    --create-namespace \
    --values "$NVML_MOCK_VALUES_FILE" \
    --wait \
    --timeout 120s >/dev/null

log "waiting for nvml-mock pods"
kubectl --context "$KUBE_CONTEXT" -n "$NVML_MOCK_NAMESPACE" wait \
    --for=condition=ready pod \
    -l app.kubernetes.io/instance="$NVML_MOCK_RELEASE" \
    --timeout=120s


log "installing Slinky operator"
helm upgrade --install slurm-operator \
    oci://ghcr.io/slinkyproject/charts/slurm-operator \
    --kube-context "$KUBE_CONTEXT" \
    --namespace "$SLINKY_OPERATOR_NAMESPACE" \
    --create-namespace \
    --set webhook.enabled=false \
    --set certManager.enabled=false \
    --set crds.enabled=true \
    --wait \
    --timeout 180s >/dev/null

log "installing Slinky Slurm cluster"
helm upgrade --install "$SLURM_RELEASE" \
    oci://ghcr.io/slinkyproject/charts/slurm \
    --kube-context "$KUBE_CONTEXT" \
    --namespace "$SLURM_NAMESPACE" \
    --create-namespace \
    --values "$SLINKY_VALUES_FILE" \
    --wait \
    --timeout 300s >/dev/null

log "waiting for Slinky worker pods"
wait_for_pods "$SLURM_NAMESPACE" "$SLURM_WORKER_SELECTOR" "Slinky worker" 300

log "installing Topograph chart"
helm upgrade --install "$TOPOGRAPH_RELEASE" charts/topograph \
    --kube-context "$KUBE_CONTEXT" \
    --namespace "$TOPOGRAPH_NAMESPACE" \
    --create-namespace \
    --values "$TOPOGRAPH_VALUES_FILE" \
    --set "image.repository=${TOPOGRAPH_IMAGE}" \
    --set "image.tag=${TOPOGRAPH_TAG}" \
    --set "node-data-broker.image.repository=${TOPOGRAPH_IMAGE}" \
    --set "node-data-broker.image.tag=${TOPOGRAPH_TAG}" \
    --set "node-observer.image.repository=${TOPOGRAPH_IMAGE}" \
    --set "node-observer.image.tag=${TOPOGRAPH_TAG}" \
    --wait \
    --timeout 180s >/dev/null

log "waiting for Topograph workloads"
kubectl --context "$KUBE_CONTEXT" -n "$TOPOGRAPH_NAMESPACE" rollout status deployment/"$TOPOGRAPH_FULLNAME" --timeout=120s
kubectl --context "$KUBE_CONTEXT" -n "$TOPOGRAPH_NAMESPACE" rollout status daemonset/"$NODE_DATA_BROKER_FULLNAME" --timeout=120s

log "validating Topograph wrote ${SLURM_TOPOLOGY_CONFIGMAP}/${SLURM_TOPOLOGY_CONFIG_KEY}"
wait_for_topology_configmap 180

log "validating Slinky topology node annotations"
wait_for_node_topology_annotations 180

log "validating Slurm topology file"
wait_for_slurm_topology_file 180

log "validating Slurm loaded topology"
wait_for_slurm_topology 180

# log "cluster summary"
# kubectl --context "$KUBE_CONTEXT" get nodes -o 'custom-columns=NAME:.metadata.name,GPU_PRESENT:.metadata.labels.nvidia\.com/gpu\.present'
# kubectl --context "$KUBE_CONTEXT" -n "$NVML_MOCK_NAMESPACE" get pods -l app.kubernetes.io/instance="$NVML_MOCK_RELEASE"
# kubectl --context "$KUBE_CONTEXT" -n "$SLINKY_OPERATOR_NAMESPACE" get pods
# kubectl --context "$KUBE_CONTEXT" -n "$SLURM_NAMESPACE" get pods -l app.kubernetes.io/instance="$SLURM_RELEASE"
# kubectl --context "$KUBE_CONTEXT" -n "$TOPOGRAPH_NAMESPACE" get pods

cat <<EOF

E2E environment is ready.

Useful commands:
  kubectl --context ${KUBE_CONTEXT} -n ${TOPOGRAPH_NAMESPACE} port-forward svc/${TOPOGRAPH_FULLNAME} 49021:49021
  curl http://127.0.0.1:49021/healthz
  kind delete cluster --name ${CLUSTER_NAME}
EOF
