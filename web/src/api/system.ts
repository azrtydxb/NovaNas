import { api } from "./client";

export type SystemInfo = {
  hostname?: string;
  version?: string;
  uptime?: string | number;
  kernel?: string;
  os?: string;
  cpu?: string;
  cores?: number;
  threads?: number;
  memory?: string | number;
  zfsVersion?: string;
  timezone?: string;
  bmc?: string;
  tpm?: string;
  secureBoot?: string;
  [k: string]: unknown;
};

export type SystemVersion = {
  version?: string;
  build?: string;
  channel?: string;
  [k: string]: unknown;
};

export type SystemTime = {
  time?: string;
  timezone?: string;
};

export type NtpConfig = {
  enabled?: boolean;
  servers?: string[];
  drift?: string;
  active?: boolean;
};

export type SystemUpdate = {
  current?: string;
  available?: string | null;
  channel?: string;
  checked?: string;
  notes?: string[];
};

export type SmtpConfig = {
  enabled?: boolean;
  host?: string;
  port?: number;
  encryption?: string;
  from?: string;
  user?: string;
  password?: string;
  lastTest?: string;
  [k: string]: unknown;
};

export const system = {
  info: () => api<SystemInfo>(`/api/v1/system/info`),
  version: () => api<SystemVersion>(`/api/v1/system/version`),
  hostname: () => api<{ hostname: string }>(`/api/v1/system/hostname`),
  setHostname: (hostname: string) =>
    api<unknown>(`/api/v1/system/hostname`, {
      method: "PUT",
      body: JSON.stringify({ hostname }),
    }),
  timezone: () => api<{ timezone: string }>(`/api/v1/system/timezone`),
  setTimezone: (timezone: string) =>
    api<unknown>(`/api/v1/system/timezone`, {
      method: "PUT",
      body: JSON.stringify({ timezone }),
    }),
  time: () => api<SystemTime>(`/api/v1/system/time`),
  ntp: () => api<NtpConfig>(`/api/v1/system/ntp`),
  updates: () => api<SystemUpdate>(`/api/v1/system/updates`),
  reboot: (reason: string) =>
    api<unknown>(`/api/v1/system/reboot`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
  shutdown: (reason: string) =>
    api<unknown>(`/api/v1/system/shutdown`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
  cancelShutdown: () =>
    api<unknown>(`/api/v1/system/cancel-shutdown`, { method: "POST" }),

  getSmtp: () => api<SmtpConfig>(`/api/v1/notifications/smtp`),
  saveSmtp: (cfg: SmtpConfig) =>
    api<unknown>(`/api/v1/notifications/smtp`, {
      method: "PUT",
      body: JSON.stringify(cfg),
    }),
  testSmtp: (to: string) =>
    api<unknown>(`/api/v1/notifications/smtp/test`, {
      method: "POST",
      body: JSON.stringify({ to }),
    }),
};
