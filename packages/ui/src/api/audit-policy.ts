import type { AuditPolicy, AuditPolicySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api } from './client';

export const auditPolicyKey = () => ['audit-policy'] as const;

export function useAuditPolicy() {
  return useQuery<AuditPolicy>({
    queryKey: auditPolicyKey(),
    queryFn: () => api.get<AuditPolicy>('/audit-policy'),
    ...QUERY_DEFAULTS,
  });
}

export function useSaveAuditPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (spec: Partial<AuditPolicySpec>) =>
      api.patch<AuditPolicy>('/audit-policy', { spec }),
    onSuccess: () => qc.invalidateQueries({ queryKey: auditPolicyKey() }),
  });
}
