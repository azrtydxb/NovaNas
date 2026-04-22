import type { TrafficPolicy, TrafficPolicySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type TrafficPolicyCreateBody = { metadata: { name: string }; spec: TrafficPolicySpec };
export type TrafficPolicyUpdateBody = { spec: Partial<TrafficPolicySpec> };

export const trafficPoliciesKey = () => ['traffic-policies'] as const;
export const trafficPolicyKey = (name: string) => ['traffic-policy', name] as const;

export function useTrafficPolicies() {
  return useQuery<TrafficPolicy[]>({
    queryKey: trafficPoliciesKey(),
    queryFn: async () => unwrapList<TrafficPolicy>(await api.get('/traffic-policies')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateTrafficPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: TrafficPolicyCreateBody) =>
      api.post<TrafficPolicy>('/traffic-policies', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: trafficPoliciesKey() }),
  });
}

export function useUpdateTrafficPolicy(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: TrafficPolicyUpdateBody) =>
      api.patch<TrafficPolicy>(`/traffic-policies/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: trafficPoliciesKey() });
      qc.invalidateQueries({ queryKey: trafficPolicyKey(name) });
    },
  });
}

export function useDeleteTrafficPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/traffic-policies/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: trafficPoliciesKey() }),
  });
}
