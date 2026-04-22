import { useWsChannel } from '@/hooks/use-ws';
import { invalidationForChannel } from '@/lib/query-invalidation';
import {
  type QueryKey,
  type UseQueryOptions,
  type UseQueryResult,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

export interface LiveQueryOptions<TData, TError>
  extends Omit<UseQueryOptions<TData, TError, TData, QueryKey>, 'queryKey' | 'queryFn'> {
  /**
   * Optional WS channel that, when an event arrives, invalidates this query
   * (and any sibling keys implied by `invalidationForChannel`).
   */
  wsChannel?: string | null;
}

/**
 * `useQuery` + WebSocket-driven cache invalidation.
 *
 * When an event arrives on `wsChannel`, the query client invalidates every
 * key implied by `invalidationForChannel(event.channel)`, causing any live
 * consumer of those keys to refetch. Since the server publishes granular
 * channels, this is much cheaper than aggressive polling.
 */
export function useLiveQuery<TData, TError = unknown>(
  queryKey: QueryKey,
  queryFn: () => Promise<TData>,
  options: LiveQueryOptions<TData, TError> = {}
): UseQueryResult<TData, TError> {
  const { wsChannel, ...queryOptions } = options;
  const qc = useQueryClient();

  useWsChannel(wsChannel ?? null, (_data, _event) => {
    if (!wsChannel) return;
    const keys = invalidationForChannel(wsChannel);
    for (const key of keys) {
      qc.invalidateQueries({ queryKey: key as unknown[] });
    }
  });

  return useQuery<TData, TError, TData, QueryKey>({
    queryKey,
    queryFn,
    ...queryOptions,
  });
}
