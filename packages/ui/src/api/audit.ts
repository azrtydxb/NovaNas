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

export interface AuditQuery {
  actor?: string;
  kind?: string;
  outcome?: 'ok' | 'warn' | 'err';
  since?: string;
  until?: string;
  limit?: number;
}

export const auditSearchKey = (q: AuditQuery) => ['audit', 'search', q] as const;

export function useAuditSearch(q: AuditQuery) {
  return useQuery<AuditEvent[]>({
    queryKey: auditSearchKey(q),
    queryFn: async () => {
      const searchParams: Record<string, string | number> = {};
      if (q.actor) searchParams.actor = q.actor;
      if (q.kind) searchParams.kind = q.kind;
      if (q.outcome) searchParams.outcome = q.outcome;
      if (q.since) searchParams.since = q.since;
      if (q.until) searchParams.until = q.until;
      searchParams.limit = q.limit ?? 100;
      return unwrapList<AuditEvent>(await api.get('/audit', { searchParams }));
    },
    ...QUERY_DEFAULTS,
  });
}
