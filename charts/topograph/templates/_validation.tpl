{{- define "topograph.validation" -}}

{{- if and .Values.ingress.enabled .Values.gatewayAPI.enabled }}
  {{- fail "ingress.enabled and gatewayAPI.enabled are mutually exclusive; deploying both routing resources against the same Service is almost always a misconfiguration. Pick one." }}
{{- end }}

{{- if and .Values.gatewayAPI.enabled (not .Values.gatewayAPI.parentRefs) }}
  {{- fail "gatewayAPI.enabled=true requires at least one entry in gatewayAPI.parentRefs referencing the existing Gateway resource this HTTPRoute should attach to. See charts/topograph/values.k8s.gateway-api-example.yaml for a complete example." }}
{{- end }}

{{- if and (eq .Values.engine.name "nfd") (hasKey (default dict .Values.env) "NFD_NAMESPACE") }}
  {{- fail "env.NFD_NAMESPACE is managed by the chart for the nfd engine; configure nfdNamespace instead" }}
{{- end }}

{{- if eq .Values.engine.name "k8s" }}
{{- $engineParams := default dict .Values.engine.params }}
{{- $acceleratorLabel := trim (toString (get $engineParams "acceleratorLabel")) }}
{{- $acceleratorDomainSourceLabel := trim (toString (get $engineParams "acceleratorDomainSourceLabel")) }}
{{- if and (ne $acceleratorLabel "") (ne $acceleratorDomainSourceLabel "") }}
  {{- fail "engine.params.acceleratorLabel and engine.params.acceleratorDomainSourceLabel cannot be set together for the k8s engine" }}
{{- end }}
{{- end }}

{{- if hasKey (default dict .Values.env) "KUBE_QPS" }}
  {{- fail "env.KUBE_QPS is managed by the chart; configure kubeClient.qps instead" }}
{{- end }}

{{- if hasKey (default dict .Values.env) "KUBE_BURST" }}
  {{- fail "env.KUBE_BURST is managed by the chart; configure kubeClient.burst instead" }}
{{- end }}

{{- if or (eq .Values.provider.name "infiniband-k8s") (eq .Values.provider.name "infiniband-bm") }}
{{- $params := default dict .Values.provider.params }}
{{- $acceleratorValue := get $params "accelerator" }}
{{- $accelerator := default dict $acceleratorValue }}
{{- $source := "none" }}
{{- if hasKey $params "accelerator" }}
{{- if and (kindIs "map" $acceleratorValue) (eq (len $acceleratorValue) 0) }}
{{- $source = "none" }}
{{- else }}
{{- $source = lower (toString (get $accelerator "source")) }}
{{- if eq $source "" }}
  {{- fail "provider.params.accelerator.source must be set when provider.params.accelerator is present" }}
{{- end }}
{{- end }}
{{- end }}

{{- if not (has $source (list "nvidia-smi" "kubernetes-label" "none")) }}
  {{- fail (printf "unsupported provider.params.accelerator.source %q" $source) }}
{{- end }}

{{- if and (eq .Values.provider.name "infiniband-k8s") (eq $source "kubernetes-label") }}
{{- $kubernetesLabel := default dict (get $accelerator "kubernetesLabel") }}
{{- $key := trim (toString (get $kubernetesLabel "key")) }}
{{- if eq $key "" }}
  {{- fail "provider.params.accelerator.kubernetesLabel.key must be set for source kubernetes-label" }}
{{- end }}
{{- end }}

{{- if and (eq .Values.provider.name "infiniband-bm") (eq $source "kubernetes-label") }}
  {{- fail "provider.params.accelerator.source kubernetes-label is not supported by infiniband-bm" }}
{{- end }}

{{- end }}

{{- if eq .Values.provider.name "gcp" }}
{{- $params := default dict .Values.provider.params }}

{{- if and
      $params.serviceAccountKeysSecret
      $params.workloadIdentityFederation }}
  {{- fail "serviceAccountKeysSecret and workloadIdentityFederation in provider.params are mutually exclusive" }}
{{- end }}

{{- if $params.workloadIdentityFederation }}
  {{- if not (and
        $params.workloadIdentityFederation.credentialsConfigmap
        $params.workloadIdentityFederation.audience) }}
    {{- fail "workloadIdentityFederation requires both credentialsConfigmap and audience" }}
  {{- end }}
{{- end }}

{{- end }}

{{- end }}
