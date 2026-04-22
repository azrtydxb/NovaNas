/**
 * Thin API client used by Playwright fixtures and specs. Mirrors the subset
 * of the NovaNas HTTP API that E2E scenarios touch; deliberately minimal to
 * avoid duplicating the generated SDK here.
 */

export interface ApiClientOptions {
  baseURL: string;
  token?: string;
}

export class ApiClient {
  readonly baseURL: string;
  private token?: string;

  constructor(opts: ApiClientOptions) {
    this.baseURL = opts.baseURL.replace(/\/$/, "");
    this.token = opts.token;
  }

  setToken(token: string): void {
    this.token = token;
  }

  private headers(extra: Record<string, string> = {}): Record<string, string> {
    const h: Record<string, string> = {
      accept: "application/json",
      "content-type": "application/json",
      ...extra,
    };
    if (this.token) h.authorization = `Bearer ${this.token}`;
    return h;
  }

  async request<T = unknown>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const res = await fetch(`${this.baseURL}${path}`, {
      method,
      headers: this.headers(),
      body: body == null ? undefined : JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(`${method} ${path} → ${res.status}: ${text}`);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }
  post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }
  put<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("PUT", path, body);
  }
  del<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  // Convenience helpers used in specs
  health() {
    return this.get<{ status: string }>("/health");
  }
  version() {
    return this.get<{ version: string; gitSha?: string }>("/api/version");
  }
  listPools() {
    return this.get<{ items: unknown[] }>("/api/v1/pools");
  }
  listDatasets() {
    return this.get<{ items: unknown[] }>("/api/v1/datasets");
  }
  listShares() {
    return this.get<{ items: unknown[] }>("/api/v1/shares");
  }
  listSnapshots() {
    return this.get<{ items: unknown[] }>("/api/v1/snapshots");
  }
  listApps() {
    return this.get<{ items: unknown[] }>("/api/v1/apps/instances");
  }
}
