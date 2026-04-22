import { useLiveQuery } from '@/hooks/use-live-query';
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
  return useLiveQuery<Alert[]>(
    alertsKey(),
    async () => unwrapList<Alert>(await api.get('/alerts', { searchParams: { state: 'active' } })),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      // Safety-net slow poll; WS delivers near-real-time updates.
      refetchInterval: 60_000,
      wsChannel: 'alert:*',
    }
  );
}
