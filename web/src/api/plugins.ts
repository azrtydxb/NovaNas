import { api } from "./client";

// Mirror of internal/plugins/manifest.go DisplayCategory enum (14 values).
export const DISPLAY_CATEGORIES = [
  "backup",
  "files",
  "multimedia",
  "photos",
  "productivity",
  "security",
  "communication",
  "home",
  "developer",
  "network",
  "storage",
  "surveillance",
  "utilities",
  "observability",
] as const;
export type DisplayCategory = (typeof DISPLAY_CATEGORIES)[number];

export type PluginIndexEntry = {
  name: string;
  displayName?: string;
  category?: string;
  displayCategory?: DisplayCategory;
  vendor?: string;
  icon?: string;
  description?: string;
  tags?: string[];
  versions: PluginIndexVersion[];
  marketplace?: string;
};

export type PluginIndexVersion = {
  version: string;
  tarballUrl: string;
  signatureUrl: string;
  sha256: string;
  size: number;
  releasedAt: string;
};

export type CategoryCount = {
  category: DisplayCategory;
  displayName: string;
  count: number;
};

export type Marketplace = {
  id: string;
  name: string;
  indexUrl: string;
  trustKeyUrl?: string;
  trustKeyPem?: string;
  trustKeyFingerprint?: string;
  locked: boolean;
  enabled: boolean;
  addedBy?: string;
  addedAt: string;
  updatedAt: string;
};

export type PluginManifest = {
  apiVersion: string;
  kind: string;
  metadata: { name: string; version: string; vendor?: string };
  spec: Record<string, unknown>;
};

export type PermissionsSummary = {
  willCreate: { kind: string; what: string; destructive: boolean }[];
  willMount: string[];
  willOpen: string[];
  scopes: string[];
  category: string;
};

export type PreviewResponse = {
  manifest: PluginManifest;
  permissions: PermissionsSummary;
};

export type ListIndexParams = {
  displayCategory?: DisplayCategory;
  tags?: string[];
  force?: boolean;
};

function qs(params: Record<string, string | string[] | boolean | undefined>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v == null) continue;
    if (Array.isArray(v)) v.forEach((x) => u.append(k, x));
    else u.append(k, String(v));
  }
  const s = u.toString();
  return s ? `?${s}` : "";
}

// Backend response shapes:
//   GET /plugins/index       → { version, updated, plugins: [...] }
//   GET /plugins/categories  → CategoryCount[]   (bare array)
//   GET /plugins             → PluginManifest[]  (bare array, when wired)
//   GET /marketplaces        → Marketplace[]     (bare array)
type PluginIndexResponse = {
  version: number;
  updated: string;
  plugins: PluginIndexEntry[];
};

export const plugins = {
  listIndex: (params: ListIndexParams = {}) =>
    api<PluginIndexResponse>(
      `/api/v1/plugins/index${qs({
        displayCategory: params.displayCategory,
        tag: params.tags,
        force: params.force,
      })}`
    ).then((r) => r.plugins),

  listCategories: () => api<CategoryCount[]>(`/api/v1/plugins/categories`),

  getManifestPreview: (name: string, version: string) =>
    api<PreviewResponse>(
      `/api/v1/plugins/index/${encodeURIComponent(name)}/manifest?version=${encodeURIComponent(version)}`
    ),

  listInstalled: () => api<PluginManifest[]>(`/api/v1/plugins`),

  listDependencies: (name: string) =>
    api<{ name: string; version: string }[]>(
      `/api/v1/plugins/${encodeURIComponent(name)}/dependencies`
    ),

  listDependents: (name: string) =>
    api<{ name: string; version: string }[]>(
      `/api/v1/plugins/${encodeURIComponent(name)}/dependents`
    ),

  restart: (name: string) =>
    api<unknown>(`/api/v1/plugins/${encodeURIComponent(name)}/restart`, {
      method: "POST",
    }),

  getLogs: (name: string, lines = 200) =>
    api<{ lines: string[] }>(
      `/api/v1/plugins/${encodeURIComponent(name)}/logs?lines=${lines}`
    ),
};

export const marketplaces = {
  list: () => api<Marketplace[]>(`/api/v1/marketplaces`),
};
