{{/*
Expand the name of the chart.
*/}}
{{- define "aws-node-retag.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Full name combining release and chart name, with fullnameOverride support.
If the release name already contains the chart name the chart name is not appended,
preventing names like "aws-node-retag-aws-node-retag".
*/}}
{{- define "aws-node-retag.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else if contains (include "aws-node-retag.name" .) .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "aws-node-retag.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
ServiceAccount name — resolves to fullname when create=true and name is empty.
*/}}
{{- define "aws-node-retag.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "aws-node-retag.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Common labels applied to every resource (standard set + user-defined commonLabels).
*/}}
{{- define "aws-node-retag.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ include "aws-node-retag.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{- toYaml . | nindent 0 }}
{{- end }}
{{- end }}

{{/*
Selector labels — stable subset used in Deployment spec.selector and matchLabels.
*/}}
{{- define "aws-node-retag.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aws-node-retag.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Fully-qualified image reference.
*/}}
{{- define "aws-node-retag.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end }}
