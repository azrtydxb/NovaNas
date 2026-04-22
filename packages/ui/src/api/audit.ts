import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type AuditTone = 'ok' | 'warn' | 'err' | 'info';

export interface AuditEvent {
  id: string;
  timestamp: string;
  actor: string;
  verb: string;
  resource?: string;
  resourceName?: string;
  message: string;
  tone?: AuditTone;
}

export const auditKey = (limit: number) => ['audit', 'recent', limit] as const;

export function useRecentAudit(limit = 20) {
  return useQuery<AuditEvent[]>({
    queryKey: auditKey(limit),
    queryFn: async () =>
      unwrapList<AuditEvent>(await api.get('/audit', { searchParams: { limit } })),
    ...QUERY_DEFAULTS,
    refetchInterval: 30_000,
  });
}
