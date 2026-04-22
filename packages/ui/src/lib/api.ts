/**
 * Minimal fetch client for the NovaNas API.
 *
 * Session auth is cookie-based — the browser handles it transparently.
 * We add CSRF / accept headers and JSON body serialization.
 */

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

export interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  searchParams?: Record<string, string | number | boolean | undefined>;
}

const DEFAULT_BASE = '/api/v1';

export function getApiBase(): string {
  return (import.meta.env.VITE_API_BASE as string | undefined) ?? DEFAULT_BASE;
}

function buildUrl(path: string, searchParams?: RequestOptions['searchParams']): string {
  const base = getApiBase();
  const joined = path.startsWith('/') ? `${base}${path}` : `${base}/${path}`;
  if (!searchParams) return joined;
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(searchParams)) {
    if (v != null) qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `${joined}?${s}` : joined;
}

export async function apiRequest<T = unknown>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const { body, searchParams, headers, ...rest } = options;
  const url = buildUrl(path, searchParams);

  const init: RequestInit = {
    credentials: 'include',
    ...rest,
    headers: {
      Accept: 'application/json',
      ...(body != null ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
    body: body != null ? JSON.stringify(body) : undefined,
  };

  const res = await fetch(url, init);
  const text = await res.text();
  const parsed: unknown = text ? safeJson(text) : null;

  if (!res.ok) {
    const message =
      (parsed && typeof parsed === 'object' && 'message' in parsed
        ? String((parsed as { message: unknown }).message)
        : null) ?? `${res.status} ${res.statusText}`;
    throw new ApiError(message, res.status, parsed);
  }

  return parsed as T;
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => apiRequest<T>(path, { ...opts, method: 'GET' }),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    apiRequest<T>(path, { ...opts, method: 'POST', body }),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    apiRequest<T>(path, { ...opts, method: 'PUT', body }),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    apiRequest<T>(path, { ...opts, method: 'PATCH', body }),
  delete: <T>(path: string, opts?: RequestOptions) =>
    apiRequest<T>(path, { ...opts, method: 'DELETE' }),
};
