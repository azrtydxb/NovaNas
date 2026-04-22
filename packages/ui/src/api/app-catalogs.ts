import type { AppCatalog } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const appCatalogsKey = () => ['app-catalogs'] as const;

export function useAppCatalogs() {
  return useQuery<AppCatalog[]>({
    queryKey: appCatalogsKey(),
    queryFn: async () => unwrapList<AppCatalog>(await api.get('/app-catalogs')),
    ...QUERY_DEFAULTS,
  });
}
