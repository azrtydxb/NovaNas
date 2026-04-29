import { api } from "./client";

export type HelmRelease = {
  release: string;
  name?: string;
  chart?: string;
  version?: string;
  appVersion?: string;
  namespace?: string;
  ns?: string;
  status?: string;
  pods?: number;
  cpu?: string | number;
  mem?: string;
  memory?: string;
  updated?: string;
  updatedAt?: string;
  [key: string]: unknown;
};

export type HelmReleaseDetail = HelmRelease & {
  values?: Record<string, unknown>;
  notes?: string;
  history?: Array<{ revision: number; updated: string; status: string; description?: string }>;
};

export type K8sEvent = {
  time?: string;
  timestamp?: string;
  t?: string;
  kind?: string;
  type?: string;
  reason?: string;
  object?: string;
  obj?: string;
  involvedObject?: string;
  message?: string;
  msg?: string;
};

export type ChartIndexEntry = {
  name: string;
  displayName?: string;
  description?: string;
  version?: string;
  appVersion?: string;
  category?: string;
  icon?: string;
  color?: string;
  installed?: boolean;
};

export const workloads = {
  list: () => api<HelmRelease[]>("/api/v1/workloads"),
  get: (name: string) =>
    api<HelmReleaseDetail>(`/api/v1/workloads/${encodeURIComponent(name)}`),
  events: (name: string) =>
    api<K8sEvent[]>(`/api/v1/workloads/${encodeURIComponent(name)}/events`),
  logs: (name: string, container?: string) => {
    const q = container ? `?container=${encodeURIComponent(container)}` : "";
    return api<string>(`/api/v1/workloads/${encodeURIComponent(name)}/logs${q}`);
  },
  rollback: (name: string, revision?: number) =>
    api<unknown>(`/api/v1/workloads/${encodeURIComponent(name)}/rollback`, {
      method: "POST",
      body: revision ? JSON.stringify({ revision }) : undefined,
    }),
  index: () => api<ChartIndexEntry[]>("/api/v1/workloads/index"),
  indexEntry: (name: string) =>
    api<ChartIndexEntry>(`/api/v1/workloads/index/${encodeURIComponent(name)}`),
};
