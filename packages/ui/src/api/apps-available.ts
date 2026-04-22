import type { App } from '@novanas/schemas';
import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export const appsAvailableKey = () => ['apps-available'] as const;

/**
 * Catalog browse — list of `App` resources synced from AppCatalogs.
 * Route: /apps-available (distinct from /apps which is AppInstance CRUD).
 */
export function useAppsAvailable() {
  return useQuery<App[]>({
    queryKey: appsAvailableKey(),
    queryFn: async () => unwrapList<App>(await api.get('/apps-available')),
    ...QUERY_DEFAULTS,
  });
}
