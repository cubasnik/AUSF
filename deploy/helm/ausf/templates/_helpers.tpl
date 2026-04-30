{{/*
Expand the name of the chart.
*/}}
{{- define "ausf.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "ausf.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Chart label.
*/}}
{{- define "ausf.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "ausf.labels" -}}
helm.sh/chart: {{ include "ausf.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Selector labels for the microservice.
*/}}
{{- define "ausf.microservice.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ausf.fullname" . }}-microservice
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the control-plane.
*/}}
{{- define "ausf.controlplane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ausf.fullname" . }}-control-plane
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for mock UDM.
*/}}
{{- define "ausf.mock.udm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ausf.fullname" . }}-mock-udm
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for mock NRF.
*/}}
{{- define "ausf.mock.nrf.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ausf.fullname" . }}-mock-nrf
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for mock AMF.
*/}}
{{- define "ausf.mock.amf.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ausf.fullname" . }}-mock-amf
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
