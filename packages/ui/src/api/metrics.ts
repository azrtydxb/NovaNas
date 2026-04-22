import { useQuery } from '@tanstack/react-query';
import { METRIC_QUERY_DEFAULTS, api } from './client';

export type MetricScope = 'pool' | 'dataset' | 'disk' | 'system';
export type MetricRange = '5m' | '15m' | '1h' | '6h' | '24h' | '7d';

export interface MetricPoint {
  t: number;
  v: number;
}

export interface MetricSeries {
  name: string;
  labels?: Record<string, string>;
  points: MetricPoint[];
}

export interface MetricResponse {
  scope: MetricScope;
  query: string;
  range: MetricRange;
  series: MetricSeries[];
}

export const metricKey = (scope: MetricScope, query: string, range: MetricRange, target?: string) =>
  ['metrics', scope, query, range, target ?? ''] as const;

export interface UseMetricOptions {
  /** Optional target resource name (pool name, dataset name, etc.). */
  target?: string;
  enabled?: boolean;
}

export function useMetric(
  scope: MetricScope,
  query: string,
  range: MetricRange,
  opts: UseMetricOptions = {}
) {
  return useQuery<MetricResponse>({
    queryKey: metricKey(scope, query, range, opts.target),
    queryFn: () =>
      api.get<MetricResponse>('/metrics', {
        searchParams: {
          scope,
          query,
          range,
          ...(opts.target ? { target: opts.target } : {}),
        },
      }),
    ...METRIC_QUERY_DEFAULTS,
    refetchInterval: 10_000,
    enabled: opts.enabled !== false,
  });
}
