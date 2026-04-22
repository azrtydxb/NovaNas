import type { HostInterface, HostInterfaceSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type HostInterfaceCreateBody = { metadata: { name: string }; spec: HostInterfaceSpec };
export type HostInterfaceUpdateBody = { spec: Partial<HostInterfaceSpec> };

export const hostInterfacesKey = () => ['host-interfaces'] as const;
export const hostInterfaceKey = (name: string) => ['host-interface', name] as const;

export function useHostInterfaces() {
  return useQuery<HostInterface[]>({
    queryKey: hostInterfacesKey(),
    queryFn: async () => unwrapList<HostInterface>(await api.get('/host-interfaces')),
    ...QUERY_DEFAULTS,
  });
}

export function useHostInterface(name: string | undefined) {
  return useQuery<HostInterface>({
    queryKey: hostInterfaceKey(name ?? ''),
    queryFn: () => api.get<HostInterface>(`/host-interfaces/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateHostInterface() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: HostInterfaceCreateBody) =>
      api.post<HostInterface>('/host-interfaces', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: hostInterfacesKey() }),
  });
}

export function useUpdateHostInterface(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: HostInterfaceUpdateBody) =>
      api.patch<HostInterface>(`/host-interfaces/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: hostInterfacesKey() });
      qc.invalidateQueries({ queryKey: hostInterfaceKey(name) });
    },
  });
}

export function useDeleteHostInterface() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/host-interfaces/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: hostInterfacesKey() }),
  });
}
