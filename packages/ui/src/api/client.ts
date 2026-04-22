/**
 * API client foundation.
 *
 * Thin wrapper on top of `@/lib/api` that carries retry/stale defaults used
 * by every hook in this package. Cookie-based session auth is handled by
 * the underlying fetch wrapper (credentials: 'include').
 */

import { ApiError, api } from '@/lib/api';

export { ApiError, api };
export type { RequestOptions } from '@/lib/api';

const IS_TEST = typeof process !== 'undefined' && process.env?.NODE_ENV === 'test';

/** Retry policy shared by list/detail queries. */
export const RETRY_POLICY = {
  /** Retry once on non-5xx failures (covers flaky 408/429), up to 3 on network errors. */
  retry: (failureCount: number, err: unknown): boolean => {
    if (IS_TEST) return false;
    if (err instanceof ApiError) {
      if (err.status >= 500) return failureCount < 3;
      return failureCount < 1;
    }
    // Network / unknown error — up to 3 attempts.
    return failureCount < 3;
  },
};

/** Shared query defaults. */
export const QUERY_DEFAULTS = {
  staleTime: 30_000,
  refetchOnWindowFocus: false,
  ...RETRY_POLICY,
} as const;

export const METRIC_QUERY_DEFAULTS = {
  staleTime: 5_000,
  refetchOnWindowFocus: false,
  ...RETRY_POLICY,
} as const;

/** Normalize a list response that may be `{ items: T[] }` or a bare array. */
export function unwrapList<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === 'object' && 'items' in value) {
    const items = (value as { items: unknown }).items;
    if (Array.isArray(items)) return items as T[];
  }
  return [];
}
