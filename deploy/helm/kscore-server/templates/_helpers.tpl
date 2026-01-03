{{/*
Expand the name of the chart.
*/}}
{{- define "kscore-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kscore-server.fullname" -}}
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
{{- define "kscore-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kscore-server.labels" -}}
helm.sh/chart: {{ include "kscore-server.chart" . }}
{{ include "kscore-server.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kscore-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kscore-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kscore-server.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kscore-server.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Determine if we're in HA mode
*/}}
{{- define "kscore-server.isHAMode" -}}
{{- if or (eq .Values.deploymentMode "ha") .Values.cluster.enabled -}}
true
{{- end -}}
{{- end }}

{{/*
Get NATS URL - handles embedded, external, and subchart modes
*/}}
{{- define "kscore-server.natsUrl" -}}
{{- if .Values.nats.url }}
{{- .Values.nats.url }}
{{- else if .Values.nats.enabled }}
{{- printf "nats://%s-nats:4222" .Release.Name }}
{{- else if eq .Values.nats.mode "embedded" }}
{{- printf "nats://localhost:%d" (int .Values.nats.embedded.port) }}
{{- else }}
{{- "nats://localhost:4222" }}
{{- end }}
{{- end }}

{{/*
Get PostgreSQL DSN
*/}}
{{- define "kscore-server.postgresqlDsn" -}}
{{- if .Values.storage.postgresql.dsn }}
{{- .Values.storage.postgresql.dsn }}
{{- else if .Values.postgresql.enabled }}
{{- printf "postgres://%s:$(POSTGRES_PASSWORD)@%s-postgresql:5432/%s?sslmode=%s" .Values.postgresql.auth.username .Release.Name .Values.postgresql.auth.database .Values.storage.postgresql.sslMode }}
{{- else if .Values.storage.postgresql.host }}
{{- printf "postgres://%s:$(POSTGRES_PASSWORD)@%s:%d/%s?sslmode=%s" .Values.storage.postgresql.username .Values.storage.postgresql.host (int .Values.storage.postgresql.port) .Values.storage.postgresql.database .Values.storage.postgresql.sslMode }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}

{{/*
Get etcd endpoints as comma-separated string
*/}}
{{- define "kscore-server.etcdEndpoints" -}}
{{- if .Values.cluster.etcd.endpoints }}
{{- .Values.cluster.etcd.endpoints | join "," }}
{{- else if .Values.etcd.enabled }}
{{- $endpoints := list }}
{{- $replicas := int .Values.etcd.replicaCount }}
{{- range $i := until $replicas }}
{{- $endpoints = append $endpoints (printf "http://%s-etcd-%d.%s-etcd-headless:2379" $.Release.Name $i $.Release.Name) }}
{{- end }}
{{- $endpoints | join "," }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}

{{/*
Get etcd endpoints as YAML list
*/}}
{{- define "kscore-server.etcdEndpointsList" -}}
{{- if .Values.cluster.etcd.endpoints }}
{{- range .Values.cluster.etcd.endpoints }}
- {{ . | quote }}
{{- end }}
{{- else if .Values.etcd.enabled }}
{{- $replicas := int .Values.etcd.replicaCount }}
{{- range $i := until $replicas }}
- {{ printf "http://%s-etcd-%d.%s-etcd-headless:2379" $.Release.Name $i $.Release.Name | quote }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Parse memory string (e.g., "1Gi") to bytes
*/}}
{{- define "kscore-server.parseMemory" -}}
{{- $mem := . }}
{{- if hasSuffix "Gi" $mem }}
{{- $val := trimSuffix "Gi" $mem | int64 }}
{{- mul $val 1073741824 }}
{{- else if hasSuffix "Mi" $mem }}
{{- $val := trimSuffix "Mi" $mem | int64 }}
{{- mul $val 1048576 }}
{{- else if hasSuffix "Ki" $mem }}
{{- $val := trimSuffix "Ki" $mem | int64 }}
{{- mul $val 1024 }}
{{- else }}
{{- $mem }}
{{- end }}
{{- end }}

{{/*
Get PostgreSQL password secret name
*/}}
{{- define "kscore-server.postgresqlSecretName" -}}
{{- if .Values.storage.postgresql.passwordSecretName }}
{{- .Values.storage.postgresql.passwordSecretName }}
{{- else if .Values.postgresql.enabled }}
{{- if .Values.postgresql.auth.existingSecret }}
{{- .Values.postgresql.auth.existingSecret }}
{{- else }}
{{- printf "%s-postgresql" .Release.Name }}
{{- end }}
{{- else }}
{{- printf "%s-postgresql" (include "kscore-server.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Get PostgreSQL password secret key
*/}}
{{- define "kscore-server.postgresqlSecretKey" -}}
{{- if .Values.storage.postgresql.passwordSecretKey }}
{{- .Values.storage.postgresql.passwordSecretKey }}
{{- else }}
{{- "postgres-password" }}
{{- end }}
{{- end }}
