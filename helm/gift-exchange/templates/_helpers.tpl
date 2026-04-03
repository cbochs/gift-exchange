{{- define "gift-exchange.name" -}}{{ .Chart.Name }}{{- end }}

{{- define "gift-exchange.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "gift-exchange.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "gift-exchange.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gift-exchange.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
