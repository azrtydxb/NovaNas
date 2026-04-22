import type { SmbServer } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const smbServersKey = () => ['smb-servers'] as const;

export function useSmbServers() {
  return useQuery<SmbServer[]>({
    queryKey: smbServersKey(),
    queryFn: async () => unwrapList<SmbServer>(await api.get('/smb-servers')),
    ...QUERY_DEFAULTS,
  });
}
