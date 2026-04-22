import type { RemoteAccessTunnel, RemoteAccessTunnelSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type RemoteAccessTunnelCreateBody = {
  metadata: { name: string };
  spec: RemoteAccessTunnelSpec;
};
export type RemoteAccessTunnelUpdateBody = { spec: Partial<RemoteAccessTunnelSpec> };

export const remoteAccessTunnelsKey = () => ['remote-access-tunnels'] as const;
export const remoteAccessTunnelKey = (name: string) => ['remote-access-tunnel', name] as const;

export function useRemoteAccessTunnels() {
  return useQuery<RemoteAccessTunnel[]>({
    queryKey: remoteAccessTunnelsKey(),
    queryFn: async () => unwrapList<RemoteAccessTunnel>(await api.get('/remote-access-tunnels')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateRemoteAccessTunnel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RemoteAccessTunnelCreateBody) =>
      api.post<RemoteAccessTunnel>('/remote-access-tunnels', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: remoteAccessTunnelsKey() }),
  });
}

export function useUpdateRemoteAccessTunnel(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RemoteAccessTunnelUpdateBody) =>
      api.patch<RemoteAccessTunnel>(`/remote-access-tunnels/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: remoteAccessTunnelsKey() });
      qc.invalidateQueries({ queryKey: remoteAccessTunnelKey(name) });
    },
  });
}

export function useDeleteRemoteAccessTunnel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api.delete<void>(`/remote-access-tunnels/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: remoteAccessTunnelsKey() }),
  });
}
