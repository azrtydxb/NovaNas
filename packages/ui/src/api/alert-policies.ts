import type { AlertPolicy, AlertPolicySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type AlertPolicyCreateBody = { metadata: { name: string }; spec: AlertPolicySpec };
export type AlertPolicyUpdateBody = { spec: Partial<AlertPolicySpec> };

export const alertPoliciesKey = () => ['alert-policies'] as const;

export function useAlertPolicies() {
  return useQuery<AlertPolicy[]>({
    queryKey: alertPoliciesKey(),
    queryFn: async () => unwrapList<AlertPolicy>(await api.get('/alert-policies')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateAlertPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AlertPolicyCreateBody) => api.post<AlertPolicy>('/alert-policies', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertPoliciesKey() }),
  });
}

export function useUpdateAlertPolicy(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AlertPolicyUpdateBody) =>
      api.patch<AlertPolicy>(`/alert-policies/${encodeURIComponent(name)}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertPoliciesKey() }),
  });
}

export function useDeleteAlertPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/alert-policies/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: alertPoliciesKey() }),
  });
}
