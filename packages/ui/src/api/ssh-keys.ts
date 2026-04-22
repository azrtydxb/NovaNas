import type { SshKey, SshKeySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type SshKeyCreateBody = { metadata: { name: string }; spec: SshKeySpec };

export const sshKeysKey = () => ['ssh-keys'] as const;

export function useSshKeys() {
  return useQuery<SshKey[]>({
    queryKey: sshKeysKey(),
    queryFn: async () => unwrapList<SshKey>(await api.get('/ssh-keys')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateSshKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: SshKeyCreateBody) => api.post<SshKey>('/ssh-keys', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: sshKeysKey() }),
  });
}

export function useDeleteSshKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/ssh-keys/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: sshKeysKey() }),
  });
}
