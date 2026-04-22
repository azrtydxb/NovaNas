import type { Vlan, VlanSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type VlanCreateBody = { metadata: { name: string }; spec: VlanSpec };
export type VlanUpdateBody = { spec: Partial<VlanSpec> };

export const vlansKey = () => ['vlans'] as const;
export const vlanKey = (name: string) => ['vlan', name] as const;

export function useVlans() {
  return useQuery<Vlan[]>({
    queryKey: vlansKey(),
    queryFn: async () => unwrapList<Vlan>(await api.get('/vlans')),
    ...QUERY_DEFAULTS,
  });
}

export function useVlan(name: string | undefined) {
  return useQuery<Vlan>({
    queryKey: vlanKey(name ?? ''),
    queryFn: () => api.get<Vlan>(`/vlans/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateVlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VlanCreateBody) => api.post<Vlan>('/vlans', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: vlansKey() }),
  });
}

export function useUpdateVlan(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VlanUpdateBody) =>
      api.patch<Vlan>(`/vlans/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: vlansKey() });
      qc.invalidateQueries({ queryKey: vlanKey(name) });
    },
  });
}

export function useDeleteVlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/vlans/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: vlansKey() }),
  });
}
