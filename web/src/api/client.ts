import { userManager } from "../auth/userManager";
import { env } from "../lib/env";

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown, message: string) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function authHeader(): Promise<HeadersInit> {
  const user = await userManager.getUser();
  if (!user || user.expired) return {};
  return { Authorization: `Bearer ${user.access_token}` };
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const url = path.startsWith("http") ? path : `${env.apiBase}${path}`;
  const headers: HeadersInit = {
    ...(init.body && !(init.body instanceof FormData)
      ? { "Content-Type": "application/json" }
      : {}),
    ...(await authHeader()),
    ...(init.headers ?? {}),
  };
  const res = await fetch(url, { ...init, headers });
  const text = await res.text();
  const body = text ? safeJSON(text) : null;
  if (!res.ok) {
    const msg =
      (body && typeof body === "object" && body !== null && "message" in body
        ? String((body as { message: unknown }).message)
        : null) ?? `${res.status} ${res.statusText}`;
    throw new ApiError(res.status, body, msg);
  }
  return body as T;
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
