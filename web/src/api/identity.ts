import { api } from "./client";

export type Me = {
  sub?: string;
  preferred_username?: string;
  username?: string;
  name?: string;
  email?: string;
  roles?: string[];
  groups?: string[];
  mfa?: boolean;
  lastLogin?: string;
  // some backends embed the raw token claims
  [key: string]: unknown;
};

export type AuthSession = {
  id: string;
  user?: string;
  username?: string;
  ip?: string;
  ipAddress?: string;
  userAgent?: string;
  client?: string;
  startedAt?: string;
  started?: string;
  current?: boolean;
  expiresAt?: string;
};

export type LoginEvent = {
  at?: string;
  timestamp?: string;
  user?: string;
  username?: string;
  ip?: string;
  method?: string;
  result?: "success" | "fail" | string;
  reason?: string;
};

export type Krb5Principal = {
  name: string;
  type?: string;
  kvno?: number;
  keyver?: number;
  created?: string;
  createdAt?: string;
  expires?: string;
  expiresAt?: string;
  enabled?: boolean;
};

export type Krb5KdcStatus = {
  online?: boolean;
  status?: string;
  realm?: string;
  kdc?: string;
  adminServer?: string;
  message?: string;
};

export type Krb5Config = {
  realm?: string;
  kdc?: string;
  adminServer?: string;
  config?: string;
  raw?: string;
  [key: string]: unknown;
};

export type Krb5IdmapdConfig = {
  domain?: string;
  config?: string;
  raw?: string;
  [key: string]: unknown;
};

export const identity = {
  me: () => api<Me>("/api/v1/auth/me"),
  sessions: () => api<AuthSession[]>("/api/v1/auth/sessions"),
  revokeSession: (id: string) =>
    api<void>(`/api/v1/auth/sessions/${encodeURIComponent(id)}`, { method: "DELETE" }),
  loginHistory: () => api<LoginEvent[]>("/api/v1/auth/login-history"),
  userSessions: (userId: string) =>
    api<AuthSession[]>(`/api/v1/auth/users/${encodeURIComponent(userId)}/sessions`),
  userLoginHistory: (userId: string) =>
    api<LoginEvent[]>(`/api/v1/auth/users/${encodeURIComponent(userId)}/login-history`),

  krb5Principals: () => api<Krb5Principal[]>("/api/v1/krb5/principals"),
  krb5Principal: (name: string) =>
    api<Krb5Principal>(`/api/v1/krb5/principals/${encodeURIComponent(name)}`),
  krb5CreatePrincipal: (body: { name: string; password?: string; type?: string }) =>
    api<Krb5Principal>("/api/v1/krb5/principals", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  krb5UpdatePrincipal: (
    name: string,
    body: { password?: string; enabled?: boolean; type?: string },
  ) =>
    api<Krb5Principal>(`/api/v1/krb5/principals/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  krb5DeletePrincipal: (name: string) =>
    api<void>(`/api/v1/krb5/principals/${encodeURIComponent(name)}`, { method: "DELETE" }),
  krb5RefreshKeytab: (name: string) =>
    api<unknown>(`/api/v1/krb5/principals/${encodeURIComponent(name)}/keytab`, {
      method: "POST",
    }),
  krb5Keytab: () => api<unknown>("/api/v1/krb5/keytab"),
  krb5KdcStatus: () => api<Krb5KdcStatus>("/api/v1/krb5/kdc/status"),
  krb5Config: () => api<Krb5Config>("/api/v1/krb5/config"),
  krb5UpdateConfig: (body: Partial<Krb5Config> & { raw?: string }) =>
    api<Krb5Config>("/api/v1/krb5/config", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  krb5Idmapd: () => api<Krb5IdmapdConfig>("/api/v1/krb5/idmapd"),
};
