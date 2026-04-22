{{/*
Shared helpers for the Grafana dashboard ConfigMaps.

"novanas.dashboard.cm" renders a ConfigMap wrapper given:
  ctx   — the root context ($)
  slug  — dashboard slug (used for uid + CM name + data key)
  json  — the rendered dashboard JSON (string)

Usage:
  {{ include "novanas.dashboard.cm" (dict
      "ctx"  $
      "slug" "system-overview"
      "json" (include "novanas.dashboard.system-overview" $)) }}
*/}}
{{- define "novanas.dashboard.cm" -}}
{{- $ctx := .ctx -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: novanas-grafana-dashboard-{{ .slug }}
  namespace: {{ include "novanas.systemNamespace" $ctx }}
  labels:
    {{- include "novanas.componentLabels" (dict "ctx" $ctx "component" "grafana-setup") | nindent 4 }}
    grafana_dashboard: "1"
  annotations:
    grafana-folder: "NovaNas"
data:
  {{ .slug }}.json: |-
{{ .json | indent 4 }}
{{- end -}}

{{/*
"novanas.dashboard.enabled" returns "true" if the given slug appears in
.Values.grafana.dashboards.list (or the list is empty, which means "all").
*/}}
{{- define "novanas.dashboard.enabled" -}}
{{- $ctx := .ctx -}}
{{- $slug := .slug -}}
{{- if not (default true (index $ctx.Values "grafana" "dashboards" "enabled")) -}}
false
{{- else if not (and $ctx.Values.observability.enabled $ctx.Values.observability.grafana.enabled) -}}
false
{{- else -}}
{{- $list := index $ctx.Values "grafana" "dashboards" "list" -}}
{{- if or (not $list) (has $slug $list) -}}true{{- else -}}false{{- end -}}
{{- end -}}
{{- end -}}
