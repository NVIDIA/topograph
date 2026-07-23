# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

KUBE_CONTEXT="${KUBE_CONTEXT:-kind-topograph}"

step() {
    if (( $# != 1 )) || [[ -z "$1" ]]; then
        echo "usage: step <command>" >&2
        return 2
    fi

    local command="$1"
    local reply

    echo
    echo "\$ $command"
    if read -rp "Run? [y/N] " reply; then
        case "$reply" in
            [yY]|[yY][eE][sS])
                eval "$command"
                ;;
        esac
    fi
}

delete_cluster() {
    kind delete cluster -n "${KUBE_CONTEXT#kind-}"
}

deploy_topograph() {
    helm upgrade --install topograph charts/topograph \
        --kube-context "$KUBE_CONTEXT" \
        --namespace topograph --create-namespace --values "$1" --wait
}
