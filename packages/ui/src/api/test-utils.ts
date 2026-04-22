import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { createElement } from 'react';

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, retryDelay: 0, staleTime: 0, gcTime: 0 },
      mutations: { retry: false, retryDelay: 0 },
    },
  });
}

export function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: qc }, children);
  };
}

export function renderHookWithClient<T>(hook: () => T) {
  const qc = makeQueryClient();
  const r = renderHook(hook, { wrapper: wrapper(qc) });
  return { ...r, qc };
}

interface MockFetchOptions {
  status?: number;
  body?: unknown;
  textBody?: string;
  /** Optional URL substring match — when provided the response is only used for matching requests. */
  match?: string;
}

export function installMockFetch() {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const queue: MockFetchOptions[] = [];

  const fetchImpl = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
    calls.push({ url, init });

    // Auto-stub for /auth/me so useAuth doesn't consume domain responses.
    if (url.includes('/auth/me')) {
      return {
        ok: false,
        status: 401,
        statusText: 'unauthorized',
        text: async () => '',
      } as unknown as Response;
    }

    // Prefer a matching queued response; fall back to the next generic one.
    const matchIdx = queue.findIndex((q) => q.match && url.includes(q.match));
    const next =
      matchIdx >= 0 ? queue.splice(matchIdx, 1)[0]! : (queue.shift() ?? { status: 200, body: {} });
    const text = next.textBody ?? (next.body !== undefined ? JSON.stringify(next.body) : '');
    return {
      ok: (next.status ?? 200) >= 200 && (next.status ?? 200) < 300,
      status: next.status ?? 200,
      statusText: 'ok',
      text: async () => text,
    } as unknown as Response;
  };

  const globalObj = globalThis as unknown as { fetch: typeof fetch };
  const previous = globalObj.fetch;
  globalObj.fetch = fetchImpl as unknown as typeof fetch;

  return {
    calls,
    enqueue(opts: MockFetchOptions) {
      queue.push(opts);
    },
    restore() {
      globalObj.fetch = previous;
    },
  };
}
