import { api } from "./client";

export type NetInterface = {
  name: string;
  type?: string;
  state?: string;
  ipv4?: string;
  ipv6?: string;
  mac?: string;
  mtu?: number;
  speed?: string;
  // tolerate alternate field names from backend
  link?: string;
  addresses?: string[];
};

export type NetConfig = {
  name: string;
  type?: string;
  enabled?: boolean;
  [k: string]: unknown;
};

export type NetBond = {
  name: string;
  mode?: string;
  members?: string[];
  state?: string;
};

export type NetVlan = {
  name: string;
  parent?: string;
  vid?: number;
  ipv4?: string;
};

export type RdmaDevice = {
  name: string;
  port?: number | string;
  state?: string;
  speed?: string;
  lid?: string | number;
  gid?: string;
};

export const network = {
  listInterfaces: () => api<NetInterface[]>(`/api/v1/network/interfaces`),
  listConfigs: () => api<NetConfig[]>(`/api/v1/network/configs`),
  getConfig: (name: string) =>
    api<NetConfig>(`/api/v1/network/configs/${encodeURIComponent(name)}`),
  createConfig: (body: NetConfig) =>
    api<NetConfig>(`/api/v1/network/configs`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateConfig: (name: string, body: NetConfig) =>
    api<NetConfig>(`/api/v1/network/configs/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteConfig: (name: string) =>
    api<unknown>(`/api/v1/network/configs/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  listBonds: () => api<NetBond[]>(`/api/v1/network/bonds`),
  createBond: (body: NetBond) =>
    api<NetBond>(`/api/v1/network/bonds`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  listVlans: () => api<NetVlan[]>(`/api/v1/network/vlans`),
  createVlan: (body: NetVlan) =>
    api<NetVlan>(`/api/v1/network/vlans`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  listRdma: () => api<RdmaDevice[]>(`/api/v1/network/rdma`),
  reload: () => api<unknown>(`/api/v1/network/reload`, { method: "POST" }),
};
