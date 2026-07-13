{{/*
Create a component fullname from the root chart fullname. Truncate the root
portion first so the component suffix remains intact within the 63-character
Kubernetes DNS-label limit.
*/}}
{{- define "nodeObserver.fullname" -}}
{{- $base := include "topograph.fullname" . | trunc 49 | trimSuffix "-" -}}
{{- printf "%s-node-observer" $base -}}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "nodeObserver.chart" -}}
{{- printf "node-observer-%s" .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nodeObserver.labels" -}}
helm.sh/chart: {{ include "nodeObserver.chart" . }}
{{ include "nodeObserver.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nodeObserver.selectorLabels" -}}
app.kubernetes.io/name: node-observer
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nodeObserver.serviceAccountName" -}}
{{- if .Values.nodeObserver.serviceAccount.create }}
{{- default (include "nodeObserver.fullname" .) .Values.nodeObserver.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.nodeObserver.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the RBAC resources.
*/}}
{{- define "nodeObserver.rbacName" -}}
{{- include "nodeObserver.fullname" . }}
{{- end }}
