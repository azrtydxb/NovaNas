import type { IsoLibrary } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const isoLibrariesKey = () => ['iso-libraries'] as const;

export function useIsoLibraries() {
  return useQuery<IsoLibrary[]>({
    queryKey: isoLibrariesKey(),
    queryFn: async () => unwrapList<IsoLibrary>(await api.get('/iso-libraries')),
    ...QUERY_DEFAULTS,
  });
}
