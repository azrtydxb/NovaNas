import { api } from "./client";

export type Pool = {
  name: string;
  state?: string;
  health?: string;
  tier?: string;
  size?: number;
  total?: number;
  used?: number;
  free?: number;
  alloc?: number;
  fragmentation?: number;
  protection?: string;
  devices?: string;
  disks?: number;
  scrubLast?: string;
  scrubNext?: string;
  throughput?: { r?: number; w?: number };
  iops?: { r?: number; w?: number };
  vdevs?: Vdev[];
};

export type Vdev = {
  name: string;
  type?: string;
  state?: string;
  disks?: string[];
  children?: Vdev[];
};

export type Dataset = {
  name: string;
  fullname?: string;
  pool?: string;
  proto?: string;
  used?: number;
  available?: number;
  quota?: number;
  referenced?: number;
  compression?: string;
  comp?: string;
  recordsize?: string;
  atime?: string;
  encryption?: string;
  enc?: boolean;
  encrypted?: boolean;
  snapshots?: number;
  snap?: number;
  mountpoint?: string;
};

export type Snapshot = {
  name: string;
  fullname?: string;
  dataset?: string;
  pool?: string;
  size?: number;
  used?: number;
  created?: string;
  schedule?: string;
  hold?: boolean;
};

export type Disk = {
  name: string;
  slot?: number;
  enclosure?: string;
  model?: string;
  serial?: string;
  size?: number;
  capacity?: number;
  cap?: number;
  state?: string;
  pool?: string;
  class?: string;
  temperature?: number;
  temp?: number;
  hours?: number;
  rotation?: number;
  smart?: { passed?: boolean; reallocated?: number; pending?: number };
};

export type SmartData = {
  passed?: boolean;
  attributes?: Record<string, unknown>;
  selftests?: unknown[];
};

export type EncryptionStatus = {
  dataset: string;
  status?: string;
  keystatus?: string;
  keyformat?: string;
  keylocation?: string;
  rotated?: string;
  encryption?: string;
};

export const storage = {
  listPools: () => api<Pool[]>(`/api/v1/pools`),
  getPool: (name: string) => api<Pool>(`/api/v1/pools/${encodeURIComponent(name)}`),
  scrubPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/scrub`, { method: "POST" }),
  syncPools: () => api<unknown>(`/api/v1/pools/sync`, { method: "POST" }),

  listDatasets: (pool?: string) =>
    api<Dataset[]>(`/api/v1/datasets${pool ? `?pool=${encodeURIComponent(pool)}` : ""}`),
  getDataset: (fullname: string) =>
    api<Dataset>(`/api/v1/datasets/${encodeURIComponent(fullname)}`),
  rollbackDataset: (fullname: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/rollback`, {
      method: "POST",
    }),

  listSnapshots: (dataset?: string) =>
    api<Snapshot[]>(
      `/api/v1/snapshots${dataset ? `?dataset=${encodeURIComponent(dataset)}` : ""}`
    ),
  createSnapshot: (dataset: string, name: string) =>
    api<Snapshot>(`/api/v1/snapshots`, {
      method: "POST",
      body: JSON.stringify({ dataset, name }),
    }),
  deleteSnapshot: (fullname: string) =>
    api<unknown>(`/api/v1/snapshots/${encodeURIComponent(fullname)}`, {
      method: "DELETE",
    }),

  listDisks: () => api<Disk[]>(`/api/v1/disks`),
  getSmart: (name: string) =>
    api<SmartData>(`/api/v1/disks/${encodeURIComponent(name)}/smart`),

  getEncryption: (fullname: string) =>
    api<EncryptionStatus>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption`),
  loadKey: (fullname: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption/load-key`, {
      method: "POST",
    }),
  unloadKey: (fullname: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption/unload-key`, {
      method: "POST",
    }),
};
