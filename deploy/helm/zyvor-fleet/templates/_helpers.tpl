{{- define "zyvor-fleet.name" -}}zyvor-fleet{{- end -}}
{{- define "zyvor-fleet.labels" -}}
app.kubernetes.io/name: {{ include "zyvor-fleet.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
