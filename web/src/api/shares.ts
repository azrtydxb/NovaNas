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

export type SmbUser = {
  username: string;
  fullname?: string;
  enabled?: boolean;
};

export type SmbGlobals = Record<string, string>;

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
  alias?: string;
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
  serial?: string;
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
  adrfam?: string;
};

export type NvmeofHost = {
  hostNqn: string;
};

const j = (b: unknown): RequestInit => ({
  method: "POST",
  body: JSON.stringify(b ?? {}),
});
const put = (b: unknown): RequestInit => ({
  method: "PUT",
  body: JSON.stringify(b ?? {}),
});

export const shares = {
  // Unified
  listProtocolShares: () => api<ProtocolShare[]>(`/api/v1/protocol-shares`),
  getProtocolShare: (name: string) =>
    api<ProtocolShare>(`/api/v1/protocol-shares/${encodeURIComponent(name)}`),

  // SMB
  listSmb: () => api<SmbShare[]>(`/api/v1/samba/shares`),
  getSmb: (name: string) =>
    api<SmbShare>(`/api/v1/samba/shares/${encodeURIComponent(name)}`),
  createSmb: (s: Partial<SmbShare>) =>
    api<SmbShare>(`/api/v1/samba/shares`, j(s)),
  updateSmb: (name: string, s: Partial<SmbShare>) =>
    api<SmbShare>(`/api/v1/samba/shares/${encodeURIComponent(name)}`, put(s)),
  deleteSmb: (name: string) =>
    api<unknown>(`/api/v1/samba/shares/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  smbReload: () => api<unknown>(`/api/v1/samba/reload`, { method: "POST" }),
  getSmbGlobals: () => api<SmbGlobals>(`/api/v1/samba/globals`),

  listSmbUsers: () => api<SmbUser[]>(`/api/v1/samba/users`),
  getSmbUser: (username: string) =>
    api<SmbUser>(`/api/v1/samba/users/${encodeURIComponent(username)}`),
  createSmbUser: (u: Partial<SmbUser> & { password?: string }) =>
    api<SmbUser>(`/api/v1/samba/users`, j(u)),
  updateSmbUser: (username: string, u: Partial<SmbUser>) =>
    api<SmbUser>(`/api/v1/samba/users/${encodeURIComponent(username)}`, put(u)),
  deleteSmbUser: (username: string) =>
    api<unknown>(`/api/v1/samba/users/${encodeURIComponent(username)}`, {
      method: "DELETE",
    }),
  setSmbUserPassword: (username: string, password: string) =>
    api<unknown>(
      `/api/v1/samba/users/${encodeURIComponent(username)}/password`,
      j({ password })
    ),

  // NFS
  listNfs: () => api<NfsExport[]>(`/api/v1/nfs/exports`),
  getNfs: (name: string) =>
    api<NfsExport>(`/api/v1/nfs/exports/${encodeURIComponent(name)}`),
  listNfsActive: () => api<NfsExport[]>(`/api/v1/nfs/exports/active`),
  createNfs: (e: Partial<NfsExport>) =>
    api<NfsExport>(`/api/v1/nfs/exports`, j(e)),
  updateNfs: (name: string, e: Partial<NfsExport>) =>
    api<NfsExport>(`/api/v1/nfs/exports/${encodeURIComponent(name)}`, put(e)),
  deleteNfs: (name: string) =>
    api<unknown>(`/api/v1/nfs/exports/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  nfsReload: () => api<unknown>(`/api/v1/nfs/reload`, { method: "POST" }),

  // iSCSI
  listIscsi: () => api<IscsiTarget[]>(`/api/v1/iscsi/targets`),
  getIscsi: (iqn: string) =>
    api<IscsiTarget>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}`),
  createIscsi: (t: Partial<IscsiTarget>) =>
    api<IscsiTarget>(`/api/v1/iscsi/targets`, j(t)),
  updateIscsi: (iqn: string, t: Partial<IscsiTarget>) =>
    api<IscsiTarget>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}`, put(t)),
  deleteIscsi: (iqn: string) =>
    api<unknown>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}`, {
      method: "DELETE",
    }),
  iscsiSaveConfig: () =>
    api<unknown>(`/api/v1/iscsi/saveconfig`, { method: "POST" }),

  listIscsiLuns: (iqn: string) =>
    api<IscsiLun[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns`),
  getIscsiLun: (iqn: string, id: string) =>
    api<IscsiLun>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns/${encodeURIComponent(id)}`
    ),
  createIscsiLun: (iqn: string, body: Partial<IscsiLun>) =>
    api<IscsiLun>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns`,
      j(body)
    ),
  updateIscsiLun: (iqn: string, id: string, body: Partial<IscsiLun>) =>
    api<IscsiLun>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns/${encodeURIComponent(id)}`,
      put(body)
    ),
  deleteIscsiLun: (iqn: string, id: string) =>
    api<unknown>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/luns/${encodeURIComponent(id)}`,
      { method: "DELETE" }
    ),

  listIscsiPortals: (iqn: string) =>
    api<IscsiPortal[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/portals`),
  createIscsiPortal: (iqn: string, body: Partial<IscsiPortal>) =>
    api<IscsiPortal>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/portals`,
      j(body)
    ),
  deleteIscsiPortal: (iqn: string, ip: string, port: number) =>
    api<unknown>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/portals/${encodeURIComponent(ip)}/${port}`,
      { method: "DELETE" }
    ),

  listIscsiAcls: (iqn: string) =>
    api<IscsiAcl[]>(`/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/acls`),
  createIscsiAcl: (iqn: string, body: Partial<IscsiAcl>) =>
    api<IscsiAcl>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/acls`,
      j(body)
    ),
  deleteIscsiAcl: (iqn: string, initiatorIqn: string) =>
    api<unknown>(
      `/api/v1/iscsi/targets/${encodeURIComponent(iqn)}/acls/${encodeURIComponent(initiatorIqn)}`,
      { method: "DELETE" }
    ),

  // NVMe-oF
  listNvmeofSubsystems: () => api<NvmeofSubsystem[]>(`/api/v1/nvmeof/subsystems`),
  getNvmeofSubsystem: (nqn: string) =>
    api<NvmeofSubsystem>(`/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}`),
  createNvmeofSubsystem: (body: Partial<NvmeofSubsystem>) =>
    api<NvmeofSubsystem>(`/api/v1/nvmeof/subsystems`, j(body)),
  updateNvmeofSubsystem: (nqn: string, body: Partial<NvmeofSubsystem>) =>
    api<NvmeofSubsystem>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}`,
      put(body)
    ),
  deleteNvmeofSubsystem: (nqn: string) =>
    api<unknown>(`/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}`, {
      method: "DELETE",
    }),
  nvmeofSaveConfig: () =>
    api<unknown>(`/api/v1/nvmeof/saveconfig`, { method: "POST" }),

  listNvmeofNamespaces: (nqn: string) =>
    api<NvmeofNamespace[]>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces`
    ),
  getNvmeofNamespace: (nqn: string, nsid: number) =>
    api<NvmeofNamespace>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces/${nsid}`
    ),
  createNvmeofNamespace: (nqn: string, body: Partial<NvmeofNamespace>) =>
    api<NvmeofNamespace>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces`,
      j(body)
    ),
  updateNvmeofNamespace: (nqn: string, nsid: number, body: Partial<NvmeofNamespace>) =>
    api<NvmeofNamespace>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces/${nsid}`,
      put(body)
    ),
  deleteNvmeofNamespace: (nqn: string, nsid: number) =>
    api<unknown>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/namespaces/${nsid}`,
      { method: "DELETE" }
    ),

  listNvmeofHosts: (nqn: string) =>
    api<NvmeofHost[]>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/hosts`
    ),
  addNvmeofHost: (nqn: string, hostNqn: string) =>
    api<unknown>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/hosts`,
      j({ hostNqn })
    ),
  removeNvmeofHost: (nqn: string, hostNqn: string) =>
    api<unknown>(
      `/api/v1/nvmeof/subsystems/${encodeURIComponent(nqn)}/hosts/${encodeURIComponent(hostNqn)}`,
      { method: "DELETE" }
    ),

  listNvmeofPorts: () => api<NvmeofPort[]>(`/api/v1/nvmeof/ports`),
  getNvmeofPort: (id: string) =>
    api<NvmeofPort>(`/api/v1/nvmeof/ports/${encodeURIComponent(id)}`),
  createNvmeofPort: (body: Partial<NvmeofPort>) =>
    api<NvmeofPort>(`/api/v1/nvmeof/ports`, j(body)),
  updateNvmeofPort: (id: string, body: Partial<NvmeofPort>) =>
    api<NvmeofPort>(`/api/v1/nvmeof/ports/${encodeURIComponent(id)}`, put(body)),
  deleteNvmeofPort: (id: string) =>
    api<unknown>(`/api/v1/nvmeof/ports/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  bindNvmeofPort: (portId: string, nqn: string) =>
    api<unknown>(
      `/api/v1/nvmeof/ports/${encodeURIComponent(portId)}/subsystems`,
      j({ nqn })
    ),
  unbindNvmeofPort: (portId: string, nqn: string) =>
    api<unknown>(
      `/api/v1/nvmeof/ports/${encodeURIComponent(portId)}/subsystems/${encodeURIComponent(nqn)}`,
      { method: "DELETE" }
    ),
  setNvmeofDhchap: (hostNqn: string, body: unknown) =>
    api<unknown>(
      `/api/v1/nvmeof/hosts/${encodeURIComponent(hostNqn)}/dhchap`,
      j(body)
    ),
};
