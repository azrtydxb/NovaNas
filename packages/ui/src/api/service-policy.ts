import type { ServicePolicy, ServicePolicySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api } from './client';

export const servicePolicyKey = () => ['service-policy'] as const;

export function useServicePolicy() {
  return useQuery<ServicePolicy>({
    queryKey: servicePolicyKey(),
    queryFn: () => api.get<ServicePolicy>('/service-policy'),
    ...QUERY_DEFAULTS,
  });
}

export function useSaveServicePolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (spec: Partial<ServicePolicySpec>) =>
      api.patch<ServicePolicy>('/service-policy', { spec }),
    onSuccess: () => qc.invalidateQueries({ queryKey: servicePolicyKey() }),
  });
}
