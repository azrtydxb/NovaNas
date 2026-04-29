import { api } from "./client";

// ─────────────────────────── Alerts (Alertmanager pass-through) ────────────────

export type AlertLabels = Record<string, string>;

export type Alert = {
  fingerprint: string;
  labels: AlertLabels;
  annotations?: Record<string, string>;
  status?: { state?: string; silencedBy?: string[]; inhibitedBy?: string[] };
  startsAt?: string;
  endsAt?: string;
  updatedAt?: string;
  generatorURL?: string;
  receivers?: { name: string }[];
};

export type AlertSilenceMatcher = {
  name: string;
  value: string;
  isRegex?: boolean;
  isEqual?: boolean;
};

export type AlertSilence = {
  id: string;
  matchers: AlertSilenceMatcher[];
  startsAt?: string;
  endsAt?: string;
  createdBy?: string;
  comment?: string;
  status?: { state?: string };
};

export type AlertReceiver = {
  name: string;
  integrations?: { name?: string; type?: string }[];
};

export const alerts = {
  list: () => api<Alert[]>(`/api/v1/alerts`),
  get: (fingerprint: string) =>
    api<Alert>(`/api/v1/alerts/${encodeURIComponent(fingerprint)}`),

  listSilences: () => api<AlertSilence[]>(`/api/v1/alert-silences`),
  createSilence: (body: Omit<AlertSilence, "id" | "status">) =>
    api<AlertSilence>(`/api/v1/alert-silences`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  expireSilence: (id: string) =>
    api<unknown>(`/api/v1/alert-silences/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  listReceivers: () => api<AlertReceiver[]>(`/api/v1/alert-receivers`),
};

// ─────────────────────────── Logs (Loki pass-through) ──────────────────────────

export type LogStream = {
  stream: Record<string, string>;
  values: [string, string][]; // [ns, line]
};

export type LogQueryResponse = {
  status?: string;
  data?: {
    resultType?: string;
    result?: LogStream[];
  };
};

export type LogLabelsResponse = { status?: string; data?: string[] };

export const logs = {
  labels: () => api<LogLabelsResponse>(`/api/v1/logs/labels`).then((r) => r.data ?? []),
  labelValues: (name: string) =>
    api<LogLabelsResponse>(
      `/api/v1/logs/labels/${encodeURIComponent(name)}/values`
    ).then((r) => r.data ?? []),

  query: (params: { query: string; start?: string; end?: string; limit?: number }) => {
    const u = new URLSearchParams();
    u.set("query", params.query);
    if (params.start) u.set("start", params.start);
    if (params.end) u.set("end", params.end);
    if (params.limit) u.set("limit", String(params.limit));
    return api<LogQueryResponse>(`/api/v1/logs/query?${u.toString()}`);
  },

  queryInstant: (query: string) =>
    api<LogQueryResponse>(
      `/api/v1/logs/query/instant?query=${encodeURIComponent(query)}`
    ),

  // TODO(phase-3): replace polling with SSE on /api/v1/logs/tail
};

// ─────────────────────────── Audit ──────────────────────────────────────────────

export type AuditEntry = {
  id?: string;
  at?: string;
  actor?: string;
  action?: string;
  resource?: string;
  result?: string;
  ip?: string;
  details?: Record<string, unknown>;
};

export type AuditSummary = {
  total?: number;
  failed?: number;
  actors?: number;
  last24h?: number;
  topActions?: { action: string; count: number }[];
};

export type AuditSearchParams = {
  limit?: number;
  actor?: string;
  action?: string;
  since?: string;
};

export const audit = {
  search: (params: AuditSearchParams = {}) => {
    const u = new URLSearchParams();
    u.set("limit", String(params.limit ?? 50));
    if (params.actor) u.set("actor", params.actor);
    if (params.action) u.set("action", params.action);
    if (params.since) u.set("since", params.since);
    return api<AuditEntry[]>(`/api/v1/audit?${u.toString()}`);
  },
  summary: () => api<AuditSummary>(`/api/v1/audit/summary`),
  exportUrl: (format: "csv" | "json" = "csv") =>
    `/api/v1/audit/export?format=${format}`,
};

// ─────────────────────────── Jobs (asynq) ──────────────────────────────────────

export type Job = {
  id: string;
  kind?: string;
  state?: string; // running | queued | ok | failed | scheduled | retry
  target?: string;
  payload?: Record<string, unknown>;
  progress?: number; // 0..1
  pct?: number;
  eta?: string;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  queue?: string;
  retried?: number;
  maxRetry?: number;
};

export type JobDetail = Job & {
  logs?: string[];
  log?: string;
};

export const jobs = {
  list: () => api<Job[]>(`/api/v1/jobs`),
  get: (id: string) => api<JobDetail>(`/api/v1/jobs/${encodeURIComponent(id)}`),
  // Cancel — backend may not support DELETE yet (returns 405). UI tolerates failure.
  cancel: (id: string) =>
    api<unknown>(`/api/v1/jobs/${encodeURIComponent(id)}`, { method: "DELETE" }),
  // Retry — backend may not support yet; POST /retry is the intended endpoint.
  retry: (id: string) =>
    api<unknown>(`/api/v1/jobs/${encodeURIComponent(id)}/retry`, {
      method: "POST",
    }),
  // TODO(phase-3): replace polling with SSE on /api/v1/jobs/{id}/stream
};

// ─────────────────────────── Notifications ─────────────────────────────────────

export type NotificationEvent = {
  id: string;
  at?: string;
  source?: string;
  title?: string;
  message?: string;
  severity?: string; // info | ok | warn | error
  actor?: string;
  read?: boolean;
  url?: string;
};

export type UnreadCount = { count: number };

export const notifications = {
  list: (params: { unread?: boolean; limit?: number } = {}) => {
    const u = new URLSearchParams();
    if (params.unread != null) u.set("unread", String(params.unread));
    u.set("limit", String(params.limit ?? 50));
    return api<NotificationEvent[]>(
      `/api/v1/notifications/events?${u.toString()}`
    );
  },
  unreadCount: () =>
    api<UnreadCount>(`/api/v1/notifications/events/unread-count`),
  readAll: () =>
    api<unknown>(`/api/v1/notifications/events/read-all`, { method: "POST" }),
  markRead: (id: string) =>
    api<unknown>(
      `/api/v1/notifications/events/${encodeURIComponent(id)}/read`,
      { method: "POST" }
    ),
  dismiss: (id: string) =>
    api<unknown>(
      `/api/v1/notifications/events/${encodeURIComponent(id)}/dismiss`,
      { method: "POST" }
    ),
  snooze: (id: string, minutes: number) =>
    api<unknown>(
      `/api/v1/notifications/events/${encodeURIComponent(id)}/snooze`,
      { method: "POST", body: JSON.stringify({ minutes }) }
    ),
  // TODO(phase-3): replace polling with SSE on /api/v1/notifications/events/stream
};
