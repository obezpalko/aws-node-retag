{{/*
Expand the name of the chart.
*/}}
{{- define "aws-node-retag.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Full name combining release and chart name.
*/}}
{{- define "aws-node-retag.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "aws-node-retag.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "aws-node-retag.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ include "aws-node-retag.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
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
