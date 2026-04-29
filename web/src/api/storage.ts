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

export type AclEntry = {
  tag?: string;
  who?: string;
  permissions?: string;
  flags?: string;
  type?: string;
};

export type DatasetMetadata = Record<string, string>;

export type Bookmark = {
  name: string;
  fullname?: string;
  created?: string;
};

const j = (b: unknown): RequestInit => ({
  method: "POST",
  body: JSON.stringify(b ?? {}),
});

export const storage = {
  // Pools
  listPools: () => api<Pool[]>(`/api/v1/pools`),
  getPool: (name: string) => api<Pool>(`/api/v1/pools/${encodeURIComponent(name)}`),
  getPoolProperties: (name: string) =>
    api<Record<string, unknown>>(`/api/v1/pools/${encodeURIComponent(name)}/properties`),
  // TODO: backend missing — POST /pools (create pool). Use import flow as workaround.
  importPool: (body: { name?: string; force?: boolean; dir?: string }) =>
    api<unknown>(`/api/v1/pools/import`, j(body)),
  syncPools: () => api<unknown>(`/api/v1/pools/sync`, { method: "POST" }),
  scrubPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/scrub`, { method: "POST" }),
  trimPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/trim`, { method: "POST" }),
  clearPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/clear`, { method: "POST" }),
  checkpointPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/checkpoint`, { method: "POST" }),
  discardCheckpoint: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/discard-checkpoint`, { method: "POST" }),
  onlineDevice: (name: string, device: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/online`, j({ device })),
  offlineDevice: (name: string, device: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/offline`, j({ device })),
  addToPool: (name: string, body: unknown) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/add`, j(body)),
  attachToPool: (name: string, body: { device: string; new_device: string }) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/attach`, j(body)),
  detachFromPool: (name: string, device: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/detach`, j({ device })),
  replaceDevice: (name: string, body: { old_device: string; new_device: string }) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/replace`, j(body)),
  upgradePool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/upgrade`, { method: "POST" }),
  exportPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/export`, { method: "POST" }),
  reguidPool: (name: string) =>
    api<unknown>(`/api/v1/pools/${encodeURIComponent(name)}/reguid`, { method: "POST" }),

  // Datasets
  listDatasets: (pool?: string) =>
    api<Dataset[]>(`/api/v1/datasets${pool ? `?pool=${encodeURIComponent(pool)}` : ""}`),
  getDataset: (fullname: string) =>
    api<Dataset>(`/api/v1/datasets/${encodeURIComponent(fullname)}`),
  // TODO: backend missing — POST /datasets (create dataset).
  rollbackDataset: (fullname: string, snapshot?: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/rollback`, j({ snapshot })),
  cloneDataset: (fullname: string, body: { snapshot: string; target: string }) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/clone`, j(body)),
  promoteDataset: (fullname: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/promote`, { method: "POST" }),
  renameDataset: (fullname: string, target: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/rename`, j({ target })),
  diffDataset: (fullname: string, body: { from?: string; to?: string }) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/diff`, j(body)),
  sendDataset: (fullname: string, body: unknown) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/send`, j(body)),
  receiveDataset: (fullname: string, body: unknown) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/receive`, j(body)),

  getAcl: (fullname: string) =>
    api<AclEntry[]>(`/api/v1/datasets/${encodeURIComponent(fullname)}/acl`),
  setAcl: (fullname: string, entries: AclEntry[]) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/acl`, j({ entries })),
  appendAcl: (fullname: string, entry: AclEntry) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/acl/append`, j(entry)),
  removeAcl: (fullname: string, index: number) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/acl/${index}`, {
      method: "DELETE",
    }),

  listBookmarks: (fullname: string) =>
    api<Bookmark[]>(`/api/v1/datasets/${encodeURIComponent(fullname)}/bookmarks`),
  createBookmark: (fullname: string, body: { snapshot: string; bookmark: string }) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/bookmark`, j(body)),
  destroyBookmark: (fullname: string, bookmark: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/destroy-bookmark`, j({ bookmark })),

  getEncryption: (fullname: string) =>
    api<EncryptionStatus>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption`),
  loadKey: (fullname: string, key?: string) =>
    api<unknown>(
      `/api/v1/datasets/${encodeURIComponent(fullname)}/encryption/load-key`,
      j(key ? { key } : {})
    ),
  unloadKey: (fullname: string) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption/unload-key`, {
      method: "POST",
    }),
  recoverKey: (fullname: string, body: unknown) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/encryption/recover`, j(body)),
  changeKey: (fullname: string, body: unknown) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/change-key`, j(body)),

  getMetadata: (fullname: string) =>
    api<DatasetMetadata>(`/api/v1/datasets/${encodeURIComponent(fullname)}/metadata`),
  putMetadata: (fullname: string, metadata: DatasetMetadata) =>
    api<unknown>(`/api/v1/datasets/${encodeURIComponent(fullname)}/metadata`, {
      method: "PUT",
      body: JSON.stringify({ metadata }),
    }),

  // Snapshots
  listSnapshots: (dataset?: string) =>
    api<Snapshot[]>(
      `/api/v1/snapshots${dataset ? `?dataset=${encodeURIComponent(dataset)}` : ""}`
    ),
  getSnapshot: (fullname: string) =>
    api<Snapshot>(`/api/v1/snapshots/${encodeURIComponent(fullname)}`),
  createSnapshot: (dataset: string, name: string) =>
    api<Snapshot>(`/api/v1/snapshots`, {
      method: "POST",
      body: JSON.stringify({ dataset, name }),
    }),
  deleteSnapshot: (fullname: string) =>
    api<unknown>(`/api/v1/snapshots/${encodeURIComponent(fullname)}`, {
      method: "DELETE",
    }),
  listHolds: (fullname: string) =>
    api<string[]>(`/api/v1/snapshots/${encodeURIComponent(fullname)}/holds`),
  holdSnapshot: (fullname: string, tag: string) =>
    api<unknown>(`/api/v1/snapshots/${encodeURIComponent(fullname)}/hold`, j({ tag })),
  releaseSnapshot: (fullname: string, tag: string) =>
    api<unknown>(`/api/v1/snapshots/${encodeURIComponent(fullname)}/release`, j({ tag })),
  getSnapshotMetadata: (fullname: string) =>
    api<DatasetMetadata>(`/api/v1/snapshots/${encodeURIComponent(fullname)}/metadata`),

  // Disks
  listDisks: () => api<Disk[]>(`/api/v1/disks`),
  getSmart: (name: string) =>
    api<SmartData>(`/api/v1/disks/${encodeURIComponent(name)}/smart`),
  enableSmart: (name: string) =>
    api<unknown>(`/api/v1/disks/${encodeURIComponent(name)}/smart/enable`, { method: "POST" }),
  startSmartTest: (name: string, type: "short" | "long" | "conveyance" | "offline") =>
    api<unknown>(`/api/v1/disks/${encodeURIComponent(name)}/smart/test`, j({ type })),
};
