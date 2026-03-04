{{- define "topograph.validation" -}}

{{- if eq .Values.global.provider.name "gcp" }}
{{- $params := .Values.global.provider.params }}

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
