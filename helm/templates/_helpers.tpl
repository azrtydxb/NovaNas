{{/*
Common helpers for the NovaNas umbrella chart.
*/}}

{{/* Chart fullname — used as the release's name prefix. */}}
{{- define "novanas.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "novanas.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "novanas.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels applied to every resource. */}}
{{- define "novanas.labels" -}}
helm.sh/chart: {{ include "novanas.chart" . }}
app.kubernetes.io/name: {{ include "novanas.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: novanas
{{- end -}}

{{/* Component-scoped selector labels.
     Usage: {{ include "novanas.selectorLabels" (dict "ctx" . "component" "api") }} */}}
{{- define "novanas.selectorLabels" -}}
app.kubernetes.io/name: {{ include "novanas.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Full per-component labels (common + selector + component). */}}
{{- define "novanas.componentLabels" -}}
{{ include "novanas.labels" .ctx }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Resolve an image reference. Expects a dict with ctx (root .) and image map.
     Falls back to .ctx.Chart.AppVersion when tag is empty. */}}
{{- define "novanas.image" -}}
{{- $registry := .ctx.Values.global.imageRegistry -}}
{{- $repo    := .image.repository -}}
{{- $tag     := default .ctx.Chart.AppVersion .image.tag -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repo $tag -}}
{{- else -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end -}}

{{/* Resolve imagePullPolicy with per-component override. */}}
{{- define "novanas.imagePullPolicy" -}}
{{- default .ctx.Values.global.imagePullPolicy .image.pullPolicy -}}
{{- end -}}

{{/* imagePullSecrets block — omitted when empty. */}}
{{- define "novanas.imagePullSecrets" -}}
{{- with .Values.global.imagePullSecrets -}}
imagePullSecrets:
{{- range . }}
  {{- if kindIs "string" . }}
  - name: {{ . }}
  {{- else }}
  - {{ toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end -}}
{{- end -}}

{{/* Default non-root security context for non-privileged NovaNas services. */}}
{{- define "novanas.securityContext" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{- define "novanas.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
runAsUser: 65532
runAsGroup: 65532
capabilities:
  drop: ["ALL"]
{{- end -}}

{{/* System namespace shorthand. */}}
{{- define "novanas.systemNamespace" -}}
{{- .Values.namespaces.system | default "novanas-system" -}}
{{- end -}}
