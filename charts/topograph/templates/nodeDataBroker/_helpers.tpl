{{/*
Create a component fullname from the root chart fullname. Truncate the root
portion first so the component suffix remains intact within the 63-character
Kubernetes DNS-label limit.
*/}}
{{- define "nodeDataBroker.fullname" -}}
{{- $base := include "topograph.fullname" . | trunc 46 | trimSuffix "-" -}}
{{- printf "%s-node-data-broker" $base -}}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "nodeDataBroker.chart" -}}
{{- printf "node-data-broker-%s" .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nodeDataBroker.labels" -}}
helm.sh/chart: {{ include "nodeDataBroker.chart" . }}
{{ include "nodeDataBroker.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nodeDataBroker.selectorLabels" -}}
app.kubernetes.io/name: node-data-broker
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nodeDataBroker.serviceAccountName" -}}
{{- if .Values.nodeDataBroker.serviceAccount.create }}
{{- default (include "nodeDataBroker.fullname" .) .Values.nodeDataBroker.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.nodeDataBroker.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the RBAC resources.
*/}}
{{- define "nodeDataBroker.rbacName" -}}
{{- include "nodeDataBroker.fullname" . }}
{{- end }}

{{/*
Create the name of a generated ConfigMap mount.
*/}}
{{- define "nodeDataBroker.configMapMountName" -}}
{{- $root := .root -}}
{{- $name := required "nodeDataBroker.configMapMounts[].name is required" .name | lower | replace "_" "-" -}}
{{- printf "%s-%s" (include "nodeDataBroker.fullname" $root) $name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the volume name for a generated ConfigMap mount.
*/}}
{{- define "nodeDataBroker.configMapMountVolumeName" -}}
{{- $name := required "nodeDataBroker.configMapMounts[].name is required" .name | lower | replace "_" "-" -}}
{{- printf "config-map-%s" $name | trunc 63 | trimSuffix "-" }}
{{- end }}
