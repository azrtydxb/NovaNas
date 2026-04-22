import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type AlertSeverity = 'info' | 'warn' | 'err';

export interface Alert {
  id: string;
  severity: AlertSeverity;
  title: string;
  message?: string;
  source?: string;
  createdAt: string;
  acknowledged?: boolean;
}

export const alertsKey = () => ['alerts', 'active'] as const;

export function useActiveAlerts() {
  return useQuery<Alert[]>({
    queryKey: alertsKey(),
    queryFn: async () =>
      unwrapList<Alert>(await api.get('/alerts', { searchParams: { state: 'active' } })),
    ...QUERY_DEFAULTS,
    refetchInterval: 30_000,
  });
}
