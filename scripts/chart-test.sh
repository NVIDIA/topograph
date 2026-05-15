#!/usr/bin/env bash
# Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

# Helm chart smoke tests: topograph umbrella chart, local subcharts
# (node-data-broker, node-observer), validation.tpl negative cases, and
# golden-file comparison for charts/topograph/values.yaml and values.*.yaml fixtures.
# Requires Helm 3.8+ on PATH.
#
# Golden outputs live under tests/charts/topograph/*.golden.yaml
# To refresh them after intentional template or values changes:
#   CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART="${ROOT}/charts/topograph"
GOLDEN_DIR="${ROOT}/tests/charts/topograph"
RELEASE="chart-ci"
NS="topograph"
KUBE_VER="${KUBE_VER:-1.30}"

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is not installed or not on PATH" >&2
  exit 1
fi

helm_common=(template "${RELEASE}" "${CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}")

assert_output_contains() {
  local haystack="$1"
  local needle="$2"
  local msg="$3"
  if ! grep -qF -- "${needle}" <<<"${haystack}"; then
    printf 'FAIL: %s (expected substring not found: %s)\n' "${msg}" "${needle}" >&2
    exit 1
  fi
}

expect_template_failure() {
  local desc="$1"
  shift
  set +e
  local out
  out=$("$@" 2>&1)
  local rc=$?
  set -e
  if [[ "${rc}" -eq 0 ]]; then
    printf 'FAIL: expected helm template to fail (%s) but it succeeded\n' "${desc}" >&2
    printf '%s\n' "${out}" >&2
    exit 1
  fi
}

assert_output_not_contains() {
  local haystack="$1"
  local needle="$2"
  local msg="$3"
  if grep -qF -- "${needle}" <<<"${haystack}"; then
    printf 'FAIL: %s (unexpected substring found: %s)\n' "${msg}" "${needle}" >&2
    exit 1
  fi
}

list_topograph_value_fixtures() {
  # Default chart values first, then every values.<suffix>.yaml example.
  printf '%s\n' "${CHART}/values.yaml"
  find "${CHART}" -maxdepth 1 -type f -name 'values.*.yaml' ! -name 'values.yaml' | LC_ALL=C sort
}

helm_template_for_fixture() {
  local values_file="$1"
  helm "${helm_common[@]}" -f "${values_file}"
}

update_golden_files() {
  echo "== CHART_TEST_UPDATE_GOLDEN: writing ${GOLDEN_DIR} =="
  mkdir -p "${GOLDEN_DIR}"
  local f base golden
  while IFS= read -r f; do
    [[ -n "${f}" ]] || continue
    base=$(basename "${f}")
    golden="${GOLDEN_DIR}/${base}.golden.yaml"
    echo "  ${base} -> ${golden}"
    helm_template_for_fixture "${f}" >"${golden}"
  done < <(list_topograph_value_fixtures)
  echo "Golden files updated. Review the diff and commit tests/charts/topograph/*.golden.yaml"
}

compare_fixture_to_golden() {
  local values_file="$1"
  local base golden
  base=$(basename "${values_file}")
  golden="${GOLDEN_DIR}/${base}.golden.yaml"

  if [[ ! -f "${golden}" ]]; then
    printf 'FAIL: missing golden file for %s (expected %s)\n' "${values_file}" "${golden}" >&2
    printf 'Create it with: CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh\n' >&2
    exit 1
  fi

  # Subshell: EXIT trap is process-wide; limiting scope ensures cleanup on return from this block
  # (success, diff mismatch, or helm_template_for_fixture failure under set -e).
  (
    actual=$(mktemp)
    trap 'rm -f "${actual}"' EXIT
    helm_template_for_fixture "${values_file}" >"${actual}"
    if ! diff -u "${golden}" "${actual}"; then
      printf '\nFAIL: helm template output for %s does not match %s\n' "${values_file}" "${golden}" >&2
      printf 'If the change is intentional, refresh goldens with: CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh\n' >&2
      exit 1
    fi
  )
}

ND_BROKER_CHART="${CHART}/charts/node-data-broker"
NODE_OBSERVER_CHART="${CHART}/charts/node-observer"
# Minimal `global` for standalone subchart lint/template (normally from parent chart).
ND_BROKER_CI_VALUES="${ROOT}/tests/charts/node-data-broker-ci.yaml"
NODE_OBSERVER_CI_VALUES="${ROOT}/tests/charts/node-observer-ci.yaml"

echo "== helm dependency build =="
helm dependency build "${CHART}" --skip-refresh

if [[ "${CHART_TEST_UPDATE_GOLDEN:-}" == "1" ]]; then
  update_golden_files
  exit 0
fi

echo "== helm lint =="
helm lint "${CHART}"

echo "== ingress enabled (not covered by example value fixtures) =="
out=$(helm "${helm_common[@]}" --set ingress.enabled=true)
assert_output_contains "${out}" "kind: Ingress" "ingress.enabled should render Ingress"

echo "== golden: values.yaml + values.*.yaml vs tests/charts/topograph/ =="
fixture_count=0
while IFS= read -r f; do
  [[ -n "${f}" ]] || continue
  echo "  compare $(basename "${f}")"
  compare_fixture_to_golden "${f}"
  fixture_count=$((fixture_count + 1))
done < <(list_topograph_value_fixtures)
if [[ "${fixture_count}" -eq 0 ]]; then
  echo "FAIL: no values.yaml / values.*.yaml fixtures found under ${CHART}" >&2
  exit 1
fi

echo "== ServiceMonitor when enabled =="
out=$(helm "${helm_common[@]}" \
  --set serviceMonitor.enabled=true \
  --api-versions=monitoring.coreos.com/v1)
assert_output_contains "${out}" "kind: ServiceMonitor" "serviceMonitor.enabled should render ServiceMonitor"

echo "== helm test hooks when tests.enabled =="
out=$(helm "${helm_common[@]}" --set tests.enabled=true)
assert_output_contains "${out}" "helm.sh/hook" "tests.enabled should emit helm test hook pods"

echo "== validation: ingress + gateway mutually exclusive =="
expect_template_failure "ingress and gateway both enabled" helm "${helm_common[@]}" \
  --set ingress.enabled=true \
  --set gatewayAPI.enabled=true \
  --set-json 'gatewayAPI.parentRefs=[{"name":"gw"}]'

echo "== validation: gateway without parentRefs =="
expect_template_failure "gateway without parentRefs" helm "${helm_common[@]}" \
  --set ingress.enabled=false \
  --set gatewayAPI.enabled=true \
  --set-json 'gatewayAPI.parentRefs=[]'

echo "== validation: GCP SA keys + WIF mutually exclusive =="
expect_template_failure "gcp SA keys and WIF together" helm "${helm_common[@]}" \
  --set global.provider.name=gcp \
  --set-json 'global.provider.params={"serviceAccountKeysSecret":"keys","workloadIdentityFederation":{"credentialsConfigmap":"cm","audience":"aud"}}'

echo "== validation: GCP WIF incomplete =="
expect_template_failure "gcp WIF missing credentialsConfigmap" helm "${helm_common[@]}" \
  --set global.provider.name=gcp \
  --set-json 'global.provider.params={"workloadIdentityFederation":{"audience":"aud"}}'

echo "== subchart node-data-broker: helm lint =="
helm lint "${ND_BROKER_CHART}" --values "${ND_BROKER_CI_VALUES}"

echo "== subchart node-data-broker: default render =="
out=$(helm template chart-ci-ndb "${ND_BROKER_CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}" \
  -f "${ND_BROKER_CI_VALUES}")
assert_output_contains "${out}" "kind: DaemonSet" "node-data-broker should render DaemonSet"
assert_output_contains "${out}" "kind: ClusterRole" "node-data-broker should render ClusterRole"
assert_output_contains "${out}" "--provider=test" "init container should pass provider name"

echo "== subchart node-data-broker: infiniband-k8s RBAC =="
out=$(helm template chart-ci-ndb "${ND_BROKER_CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}" \
  -f "${ND_BROKER_CI_VALUES}" \
  --set global.provider.name=infiniband-k8s)
assert_output_contains "${out}" "pods/exec" "infiniband-k8s should add pods/exec RBAC rule"

echo "== subchart node-data-broker: enabled=false =="
out=$(helm template chart-ci-ndb "${ND_BROKER_CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}" \
  -f "${ND_BROKER_CI_VALUES}" \
  --set enabled=false)
assert_output_not_contains "${out}" "kind: DaemonSet" "enabled=false should not render DaemonSet"

echo "== subchart node-data-broker: initc.enabled=false =="
out=$(helm template chart-ci-ndb "${ND_BROKER_CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}" \
  -f "${ND_BROKER_CI_VALUES}" \
  --set initc.enabled=false)
assert_output_not_contains "${out}" "init-node-labels" "initc.enabled=false should omit init container"

echo "== subchart node-observer: helm lint =="
helm lint "${NODE_OBSERVER_CHART}" --values "${NODE_OBSERVER_CI_VALUES}"

echo "== subchart node-observer: default render =="
out=$(helm template chart-ci-nob "${NODE_OBSERVER_CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}" \
  -f "${NODE_OBSERVER_CI_VALUES}")
assert_output_contains "${out}" "kind: Deployment" "node-observer should render Deployment"
assert_output_contains "${out}" "kind: ConfigMap" "node-observer should render ConfigMap"
assert_output_contains "${out}" "generateTopologyUrl" "config should declare generateTopologyUrl"
assert_output_contains "${out}" "/usr/local/bin/node-observer" "main container should run node-observer"

echo "All chart tests passed."
