import { useLiveQuery } from '@/hooks/use-live-query';
import type { StoragePool, StoragePoolSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type PoolCreateBody = {
  metadata: { name: string };
  spec: StoragePoolSpec;
};

export type PoolUpdateBody = {
  spec: Partial<StoragePoolSpec>;
};

export const poolsKey = () => ['pools'] as const;
export const poolKey = (name: string) => ['pool', name] as const;

export function usePools() {
  return useLiveQuery<StoragePool[]>(
    poolsKey(),
    async () => unwrapList<StoragePool>(await api.get('/pools')),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'pool:*' }
  );
}

export function usePool(name: string | undefined) {
  return useLiveQuery<StoragePool>(
    poolKey(name ?? ''),
    () => api.get<StoragePool>(`/pools/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `pool:${name}` : null,
    }
  );
}

export function useCreatePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: PoolCreateBody) => api.post<StoragePool>('/pools', body),
    onSuccess: (pool) => {
      qc.invalidateQueries({ queryKey: poolsKey() });
      if (pool?.metadata?.name) qc.setQueryData(poolKey(pool.metadata.name), pool);
    },
  });
}

export function useUpdatePool(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: PoolUpdateBody) =>
      api.patch<StoragePool>(`/pools/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: poolsKey() });
      qc.invalidateQueries({ queryKey: poolKey(name) });
    },
  });
}

export function useDeletePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/pools/${encodeURIComponent(name)}`),
    onSuccess: (_data, name) => {
      qc.invalidateQueries({ queryKey: poolsKey() });
      qc.removeQueries({ queryKey: poolKey(name) });
    },
  });
}
