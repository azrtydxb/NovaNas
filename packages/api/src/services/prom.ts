import type { Redis } from 'ioredis';
import type { Env } from '../env.js';

/**
 * Prometheus query client. Talks to the Prometheus HTTP API:
 *   - `/api/v1/query`       — instant query
 *   - `/api/v1/query_range` — ranged query
 *
 * Results are cached in Redis. Live ranges (range ending within the last
 * 30 seconds) get a 10s TTL; historical ranges get 60s.
 */

export interface PromPoint {
  t: number; // unix seconds
  v: number;
}

export interface PromSeries {
  labels: Record<string, string>;
  points: PromPoint[];
}

export interface PromQueryRange {
  start: Date;
  end: Date;
  stepSeconds: number;
}

export interface PromClient {
  readonly url: string | undefined;
  queryInstant(expr: string): Promise<PromSeries[]>;
  queryRange(expr: string, range: PromQueryRange): Promise<PromSeries[]>;
  /** Convenience combining range parsing + cache. */
  query(expr: string, range: PromQueryRange): Promise<PromSeries[]>;
}

interface PromRangeResp {
  status: 'success' | 'error';
  error?: string;
  data?: {
    resultType: string;
    result: Array<{ metric: Record<string, string>; values: Array<[number, string]> }>;
  };
}

interface PromInstantResp {
  status: 'success' | 'error';
  error?: string;
  data?: {
    resultType: string;
    result: Array<{ metric: Record<string, string>; value: [number, string] }>;
  };
}

export interface CreatePromOptions {
  fetchImpl?: typeof fetch;
  redis?: Redis | null;
}

export function createPromClient(env: Env, options: CreatePromOptions = {}): PromClient {
  const base = env.PROMETHEUS_URL;
  const fetchImpl = options.fetchImpl ?? globalThis.fetch;
  const redis = options.redis ?? null;

  async function cacheGet(key: string): Promise<PromSeries[] | null> {
    if (!redis) return null;
    try {
      const raw = await redis.get(key);
      if (!raw) return null;
      return JSON.parse(raw) as PromSeries[];
    } catch {
      return null;
    }
  }
  async function cacheSet(key: string, ttl: number, value: PromSeries[]): Promise<void> {
    if (!redis) return;
    try {
      await redis.setex(key, ttl, JSON.stringify(value));
    } catch {
      /* ignore */
    }
  }

  function ensureBase(): string {
    if (!base) throw new Error('prometheus_url_not_configured');
    return base.replace(/\/+$/, '');
  }

  async function queryInstant(expr: string): Promise<PromSeries[]> {
    const url = `${ensureBase()}/api/v1/query?query=${encodeURIComponent(expr)}`;
    const res = await fetchImpl(url);
    if (!res.ok) throw new Error(`prom_query_failed:${res.status}`);
    const body = (await res.json()) as PromInstantResp;
    if (body.status !== 'success' || !body.data) {
      throw new Error(body.error ?? 'prom_query_error');
    }
    return body.data.result.map((r) => ({
      labels: r.metric,
      points: [{ t: r.value[0], v: Number(r.value[1]) }],
    }));
  }

  async function queryRange(expr: string, range: PromQueryRange): Promise<PromSeries[]> {
    const params = new URLSearchParams({
      query: expr,
      start: String(Math.floor(range.start.getTime() / 1000)),
      end: String(Math.floor(range.end.getTime() / 1000)),
      step: String(range.stepSeconds),
    });
    const url = `${ensureBase()}/api/v1/query_range?${params.toString()}`;
    const res = await fetchImpl(url);
    if (!res.ok) throw new Error(`prom_query_range_failed:${res.status}`);
    const body = (await res.json()) as PromRangeResp;
    if (body.status !== 'success' || !body.data) {
      throw new Error(body.error ?? 'prom_query_range_error');
    }
    return body.data.result.map((r) => ({
      labels: r.metric,
      points: r.values.map(([t, v]) => ({ t, v: Number(v) })),
    }));
  }

  async function query(expr: string, range: PromQueryRange): Promise<PromSeries[]> {
    const endSec = Math.floor(range.end.getTime() / 1000);
    const startSec = Math.floor(range.start.getTime() / 1000);
    const nowSec = Math.floor(Date.now() / 1000);
    const isLive = nowSec - endSec <= 30;
    const ttl = isLive ? 10 : 60;
    const cacheKey = `novanas:prom:${expr}|${startSec}|${endSec}|${range.stepSeconds}`;
    const cached = await cacheGet(cacheKey);
    if (cached) return cached;
    const result = await queryRange(expr, range);
    await cacheSet(cacheKey, ttl, result);
    return result;
  }

  return {
    url: base,
    queryInstant,
    queryRange,
    query,
  };
}
