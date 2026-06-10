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
# golden-file comparison for tracked values fixtures.
# Requires Helm 3.10+ on PATH.
#
# Golden outputs:
#   tests/charts/*.golden.yaml — full umbrella render
# To refresh after intentional template or values changes:
#   CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART="${ROOT}/charts/topograph"
RELEASE="chart-ci"
NS="topograph"
KUBE_VER="${KUBE_VER:-1.30}"

TOPOGRAPH_GOLDEN_DIR="${ROOT}/tests/charts"

helm_common=(template "${RELEASE}" "${CHART}" --namespace "${NS}" --kube-version "${KUBE_VER}")

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is not installed or not on PATH" >&2
  exit 1
fi

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_output_contains() {
  local haystack="$1"
  local needle="$2"
  local msg="$3"
  if ! grep -qF -- "${needle}" <<<"${haystack}"; then
    fail "${msg} (expected substring not found: ${needle})"
  fi
}

assert_output_not_contains() {
  local haystack="$1"
  local needle="$2"
  local msg="$3"
  if grep -qF -- "${needle}" <<<"${haystack}"; then
    fail "${msg} (unexpected substring found: ${needle})"
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

is_git_worktree() {
  git -C "${ROOT}" rev-parse --is-inside-work-tree >/dev/null 2>&1
}

list_values_files() {
  local dir="$1"
  local name_pattern="$2"

  if is_git_worktree; then
    local rel_dir
    rel_dir="${dir#"${ROOT}/"}"
    git -C "${ROOT}" ls-files "${rel_dir}/${name_pattern}" | while IFS= read -r rel; do
      case "${rel}" in
        *.golden.yaml) ;;
        *) printf '%s/%s\n' "${ROOT}" "${rel}" ;;
      esac
    done | LC_ALL=C sort
  else
    find "${dir}" -maxdepth 1 -type f -name "${name_pattern}" ! -name '*.golden.yaml' | LC_ALL=C sort
  fi
}

list_topograph_value_fixtures() {
  printf '%s\n' "${CHART}/values.yaml"
  list_values_files "${CHART}" 'values.*.yaml'
}

helm_template_topograph_for_fixture() {
  helm "${helm_common[@]}" -f "$1"
}

golden_path_for_fixture() {
  local golden_dir="$1"
  local values_file="$2"
  printf '%s/%s.golden.yaml\n' "${golden_dir}" "$(basename "${values_file}")"
}

compare_render_to_golden() {
  local values_file="$1"
  local golden_dir="$2"
  local renderer="$3"
  local golden
  golden="$(golden_path_for_fixture "${golden_dir}" "${values_file}")"

  if [[ ! -f "${golden}" ]]; then
    printf 'FAIL: missing golden file for %s (expected %s)\n' "${values_file}" "${golden}" >&2
    printf 'Create it with: CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh\n' >&2
    exit 1
  fi

  (
    actual=$(mktemp)
    trap 'rm -f "${actual}"' EXIT
    "${renderer}" "${values_file}" >"${actual}"
    if ! diff -u "${golden}" "${actual}"; then
      printf '\nFAIL: rendered output for %s does not match %s\n' "${values_file}" "${golden}" >&2
      printf 'If the change is intentional, refresh goldens with: CHART_TEST_UPDATE_GOLDEN=1 scripts/chart-test.sh\n' >&2
      exit 1
    fi
  )
}

compare_golden_set() {
  local label="$1"
  local golden_dir="$2"
  local lister="$3"
  local renderer="$4"
  local count=0
  local f

  echo "== golden: ${label} =="
  while IFS= read -r f; do
    [[ -n "${f}" ]] || continue
    echo "  compare $(basename "${f}")"
    compare_render_to_golden "${f}" "${golden_dir}" "${renderer}"
    count=$((count + 1))
  done < <("${lister}")

  if [[ "${count}" -eq 0 ]]; then
    fail "no ${label} values fixtures found"
  fi
}

update_golden_set() {
  local label="$1"
  local golden_dir="$2"
  local lister="$3"
  local renderer="$4"
  local count=0
  local f golden

  echo "== CHART_TEST_UPDATE_GOLDEN: writing ${label} goldens =="
  mkdir -p "${golden_dir}"
  while IFS= read -r f; do
    [[ -n "${f}" ]] || continue
    golden="$(golden_path_for_fixture "${golden_dir}" "${f}")"
    echo "  $(basename "${f}") -> ${golden}"
    "${renderer}" "${f}" >"${golden}"
    count=$((count + 1))
  done < <("${lister}")

  if [[ "${count}" -eq 0 ]]; then
    fail "no ${label} values fixtures found"
  fi
}

update_all_golden_files() {
  update_golden_set "topograph" "${TOPOGRAPH_GOLDEN_DIR}" \
    list_topograph_value_fixtures helm_template_topograph_for_fixture
  echo "Golden files updated. Review the diff under tests/charts/ before committing."
}

if [[ "${CHART_TEST_UPDATE_GOLDEN:-}" == "1" ]]; then
  update_all_golden_files
  exit 0
fi

echo "== helm lint =="
helm lint "${CHART}"

echo "== ingress enabled (not covered by example value fixtures) =="
out=$(helm "${helm_common[@]}" --set ingress.enabled=true)
assert_output_contains "${out}" "kind: Ingress" "ingress.enabled should render Ingress"

compare_golden_set "topograph values.yaml + values.*.yaml" "${TOPOGRAPH_GOLDEN_DIR}" \
  list_topograph_value_fixtures helm_template_topograph_for_fixture

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

echo "== subchart node-data-broker: enabled=false =="
out=$(helm "${helm_common[@]}" --set node-data-broker.enabled=false)
assert_output_not_contains "${out}" "# Source: topograph/charts/node-data-broker/" "node-data-broker.enabled=false should not render node-data-broker manifests"

echo "== subchart node-data-broker: initc.enabled=false =="
out=$(helm "${helm_common[@]}" --set node-data-broker.initc.enabled=false)
assert_output_not_contains "${out}" "init-node-labels" "node-data-broker.initc.enabled=false should omit init container"

echo "== subchart node-data-broker: configMapMounts =="
out=$(helm "${helm_common[@]}" \
  --set-json 'node-data-broker.configMapMounts=[{"name":"ibdiag","mountPath":"/etc/infiniband-diags/ibdiag.conf","subPath":"ibdiag.conf","data":{"ibdiag.conf":"CA=smi0\nPort=1"}}]')
assert_output_contains "${out}" "name: chart-ci-node-data-broker-ibdiag" "node-data-broker.configMapMounts should render an ibdiag ConfigMap"
assert_output_contains "${out}" "    CA=smi0" "node-data-broker configMapMounts should include configured CA"
assert_output_contains "${out}" "    Port=1" "node-data-broker configMapMounts should include configured Port"
assert_output_contains "${out}" "mountPath: \"/etc/infiniband-diags/ibdiag.conf\"" "node-data-broker configMapMounts should mount at ibdiag.conf"
assert_output_contains "${out}" "subPath: \"ibdiag.conf\"" "node-data-broker configMapMounts should mount as a single file"

echo "== subchart node-observer: replicaCount override =="
out=$(helm "${helm_common[@]}" --set node-observer.replicaCount=2)
assert_output_contains "${out}" "replicas: 2" "node-observer.replicaCount=2 should scale Deployment"

echo "All chart tests passed."
