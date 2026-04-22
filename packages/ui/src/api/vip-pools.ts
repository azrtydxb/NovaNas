import type { VipPool, VipPoolSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type VipPoolCreateBody = { metadata: { name: string }; spec: VipPoolSpec };
export type VipPoolUpdateBody = { spec: Partial<VipPoolSpec> };

export const vipPoolsKey = () => ['vip-pools'] as const;
export const vipPoolKey = (name: string) => ['vip-pool', name] as const;

export function useVipPools() {
  return useQuery<VipPool[]>({
    queryKey: vipPoolsKey(),
    queryFn: async () => unwrapList<VipPool>(await api.get('/vip-pools')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateVipPool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VipPoolCreateBody) => api.post<VipPool>('/vip-pools', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: vipPoolsKey() }),
  });
}

export function useUpdateVipPool(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VipPoolUpdateBody) =>
      api.patch<VipPool>(`/vip-pools/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: vipPoolsKey() });
      qc.invalidateQueries({ queryKey: vipPoolKey(name) });
    },
  });
}

export function useDeleteVipPool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/vip-pools/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: vipPoolsKey() }),
  });
}
