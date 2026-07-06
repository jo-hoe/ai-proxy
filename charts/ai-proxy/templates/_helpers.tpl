{{- define "ai-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "ai-proxy.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "ai-proxy.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "ai-proxy.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
app.kubernetes.io/name: {{ include "ai-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "ai-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ai-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Resolved secret name for the OIDC token — either an externally-provided
existingSecret or the chart-rendered one.
*/}}
{{- define "ai-proxy.tokenSecretName" -}}
{{- if .Values.oidc.existingSecret -}}
{{- .Values.oidc.existingSecret -}}
{{- else -}}
{{- printf "%s-token" (include "ai-proxy.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
Resolved ServiceAccount name. Uses the explicit serviceAccount.name when
set, otherwise falls back to the fullname.
*/}}
{{- define "ai-proxy.serviceAccountName" -}}
{{- default (include "ai-proxy.fullname" .) .Values.serviceAccount.name -}}
{{- end -}}
