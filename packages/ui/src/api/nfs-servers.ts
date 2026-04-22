import type { NfsServer } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const nfsServersKey = () => ['nfs-servers'] as const;

export function useNfsServers() {
  return useQuery<NfsServer[]>({
    queryKey: nfsServersKey(),
    queryFn: async () => unwrapList<NfsServer>(await api.get('/nfs-servers')),
    ...QUERY_DEFAULTS,
  });
}
