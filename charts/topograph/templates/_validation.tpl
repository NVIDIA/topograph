{{- define "topograph.validation" -}}

{{- if and .Values.ingress.enabled .Values.gatewayAPI.enabled }}
  {{- fail "ingress.enabled and gatewayAPI.enabled are mutually exclusive; deploying both routing resources against the same Service is almost always a misconfiguration. Pick one." }}
{{- end }}

{{- if and .Values.gatewayAPI.enabled (not .Values.gatewayAPI.parentRefs) }}
  {{- fail "gatewayAPI.enabled=true requires at least one entry in gatewayAPI.parentRefs referencing the existing Gateway resource this HTTPRoute should attach to. See charts/topograph/values.k8s.gateway-api-example.yaml for a complete example." }}
{{- end }}

{{- if eq .Values.global.provider.name "gcp" }}
{{- $params := default dict .Values.global.provider.params }}

{{- if and
      $params.serviceAccountKeysSecret
      $params.workloadIdentityFederation }}
  {{- fail "serviceAccountKeysSecret and workloadIdentityFederation in global.provider.params are mutually exclusive" }}
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
