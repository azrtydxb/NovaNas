import { api, ApiError } from "./client";
import { env } from "../lib/env";

export type VM = {
  name: string;
  namespace: string;
  ns?: string;
  os?: string;
  state?: "Running" | "Paused" | "Stopped" | "Pending" | string;
  status?: string;
  ip?: string;
  uptime?: string;
  cpu?: number;
  vcpu?: number;
  ram?: number; // MiB
  memory?: number;
  disk?: string | number;
  mac?: string;
  node?: string;
  [key: string]: unknown;
};

export type VMTemplate = {
  name: string;
  namespace?: string;
  os?: string;
  cpu?: number;
  ram?: number;
  disk?: number;
  source?: string;
  [key: string]: unknown;
};

export type VMSnapshot = {
  name: string;
  namespace?: string;
  vm?: string;
  vmName?: string;
  size?: string | number;
  created?: string;
  createdAt?: string;
  t?: string;
  [key: string]: unknown;
};

export type VMRestore = {
  name: string;
  namespace?: string;
  snapshotName?: string;
  vm?: string;
  status?: string;
  created?: string;
};

function action(namespace: string, name: string, verb: string, body?: unknown) {
  return api<unknown>(
    `/api/v1/vms/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/${verb}`,
    {
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
    },
  );
}

export const vms = {
  list: () => api<VM[]>("/api/v1/vms"),
  get: (namespace: string, name: string) =>
    api<VM>(`/api/v1/vms/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`),
  start: (ns: string, name: string) => action(ns, name, "start"),
  stop: (ns: string, name: string) => action(ns, name, "stop"),
  restart: (ns: string, name: string) => action(ns, name, "restart"),
  pause: (ns: string, name: string) => action(ns, name, "pause"),
  unpause: (ns: string, name: string) => action(ns, name, "unpause"),
  migrate: (ns: string, name: string, targetNode?: string) =>
    action(ns, name, "migrate", targetNode ? { targetNode } : undefined),
  consoleUrl: (ns: string, name: string) =>
    `${env.apiBase}/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/console`,
  serialUrl: (ns: string, name: string) =>
    `${env.apiBase}/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/serial`,

  templates: () => api<VMTemplate[]>("/api/v1/vm-templates"),
  createFromTemplate: (body: { name: string; template: string; namespace?: string }) =>
    api<unknown>("/api/v1/vms", { method: "POST", body: JSON.stringify(body) }),

  snapshots: () => api<VMSnapshot[]>("/api/v1/vm-snapshots"),
  createSnapshot: (body: { name: string; vmName: string; namespace?: string }) =>
    api<unknown>("/api/v1/vm-snapshots", { method: "POST", body: JSON.stringify(body) }),
  deleteSnapshot: (ns: string, name: string) =>
    api<void>(
      `/api/v1/vm-snapshots/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    ),

  restores: () => api<VMRestore[]>("/api/v1/vm-restores"),
  restore: (snapshotName: string, namespace?: string) =>
    api<unknown>(`/api/v1/vm-restores`, {
      method: "POST",
      body: JSON.stringify({ snapshotName, namespace }),
    }),
  deleteRestore: (ns: string, name: string) =>
    api<void>(
      `/api/v1/vm-restores/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    ),
};

export { ApiError };
