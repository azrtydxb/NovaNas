import type { Bond, BondSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type BondCreateBody = { metadata: { name: string }; spec: BondSpec };
export type BondUpdateBody = { spec: Partial<BondSpec> };

export const bondsKey = () => ['bonds'] as const;
export const bondKey = (name: string) => ['bond', name] as const;

export function useBonds() {
  return useQuery<Bond[]>({
    queryKey: bondsKey(),
    queryFn: async () => unwrapList<Bond>(await api.get('/bonds')),
    ...QUERY_DEFAULTS,
  });
}

export function useBond(name: string | undefined) {
  return useQuery<Bond>({
    queryKey: bondKey(name ?? ''),
    queryFn: () => api.get<Bond>(`/bonds/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateBond() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BondCreateBody) => api.post<Bond>('/bonds', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: bondsKey() }),
  });
}

export function useUpdateBond(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BondUpdateBody) =>
      api.patch<Bond>(`/bonds/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bondsKey() });
      qc.invalidateQueries({ queryKey: bondKey(name) });
    },
  });
}

export function useDeleteBond() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/bonds/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: bondsKey() }),
  });
}
