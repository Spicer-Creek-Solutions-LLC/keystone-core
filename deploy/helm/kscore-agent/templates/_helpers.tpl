{{/*
Expand the name of the chart.
*/}}
{{- define "kscore-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kscore-agent.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kscore-agent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kscore-agent.labels" -}}
helm.sh/chart: {{ include "kscore-agent.chart" . }}
{{ include "kscore-agent.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: agent
app.kubernetes.io/part-of: keystone-core
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kscore-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kscore-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kscore-agent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kscore-agent.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Generate NATS URL
*/}}
{{- define "kscore-agent.natsUrl" -}}
{{- if .Values.nats.url }}
{{- .Values.nats.url }}
{{- else }}
{{- printf "nats://%s:4222" .Values.server.address | replace ":9090" "" }}
{{- end }}
{{- end }}

{{/*
DNS Policy based on hostNetwork
*/}}
{{- define "kscore-agent.dnsPolicy" -}}
{{- if .Values.dnsPolicy }}
{{- .Values.dnsPolicy }}
{{- else if .Values.hostNetwork }}
ClusterFirstWithHostNet
{{- else }}
ClusterFirst
{{- end }}
{{- end }}
