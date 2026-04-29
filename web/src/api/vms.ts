import { api } from "./client";

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

function action(namespace: string, name: string, verb: string) {
  return api<unknown>(
    `/api/v1/vms/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/${verb}`,
    { method: "POST" },
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
  templates: () => api<VMTemplate[]>("/api/v1/vm-templates"),
  snapshots: () => api<VMSnapshot[]>("/api/v1/vm-snapshots"),
  restore: (snapshotName: string, namespace?: string) =>
    api<unknown>(`/api/v1/vm-restores`, {
      method: "POST",
      body: JSON.stringify({ snapshotName, namespace }),
    }),
};
