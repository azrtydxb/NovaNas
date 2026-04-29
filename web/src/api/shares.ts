import { api } from "./client";

export type ProtocolShare = {
  name: string;
  protocols?: string[];
  path?: string;
  clients?: string;
  state?: string;
};

export type SmbShare = {
  name: string;
  path?: string;
  users?: string;
  guest?: boolean;
  recycle?: boolean;
  vfs?: string;
  comment?: string;
  readOnly?: boolean;
};

export type NfsExport = {
  name: string;
  path?: string;
  clients?: string;
  options?: string;
  active?: boolean;
};

export type IscsiTarget = {
  iqn: string;
  luns?: number;
  portals?: string[];
  acls?: number;
  state?: string;
};

export type IscsiLun = {
  id: string;
  lun?: number;
  backing?: string;
  size?: number;
};

export type IscsiPortal = {
  id?: string;
  ip?: string;
  port?: number;
  tag?: number;
};

export type IscsiAcl = {
  initiator: string;
  user?: string;
  authMethod?: string;
};

export type NvmeofSubsystem = {
  nqn: string;
  ns?: number;
  ports?: number;
  hosts?: number;
  dhchap?: boolean;
  state?: string;
};

export type NvmeofNamespace = {
  nsid: number;
  device?: string;
  size?: number;
  uuid?: string;
};

export type NvmeofPort = {
  id: string;
  trtype?: string;
  trsvcid?: string;
  traddr?: string;
};

export const shares = {
  listProtocolShares: () => api<ProtocolShare[]>(`/api/v1/protocol-shares`),

  listSmb: () => api<SmbShare[]>(`/api/v1/samba/shares`),
  createSmb: (s: Partial<SmbShare>) =>
    api<SmbShare>(`/api/v1/samba/shares`, {
      method: "POST",
      body: JSON.stringify(s),
    }),
  deleteSmb: (name: string) =>
    api<unknown>(`/api/v1/samba/shares/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  listNfs: () => api<NfsExport[]>(`/api/v1/nfs/exports`),
  createNfs: (e: Partial<NfsExport>) =>
    api<NfsExport>(`/api/v1/nfs/exports`, {
      method: "POST",
      body: JSON.stringify(e),
    }),
  deleteNfs: (name: string) =>
    api<unknown>(`/api/v1/nfs/exports/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  listIscsi: () => api<IscsiTarget[]>(`/api/v1/iscsi/targets`),
  listIscsiLuns: (iqn: string) =>
    api<IscsiLun[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns`),
  listIscsiPortals: (iqn: string) =>
    api<IscsiPortal[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/portals`),
  listIscsiAcls: (iqn: string) =>
    api<IscsiAcl[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/acls`),

  listNvmeofSubsystems: () => api<NvmeofSubsystem[]>(`/api/v1/nvmeof/subsystems`),
  listNvmeofNamespaces: (nqn: string) =>
    api<NvmeofNamespace[]>(`/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces`),
  listNvmeofPorts: () => api<NvmeofPort[]>(`/api/v1/nvmeof/ports`),
};
