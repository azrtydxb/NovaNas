import type { CustomDomain, CustomDomainSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type CustomDomainCreateBody = { metadata: { name: string }; spec: CustomDomainSpec };
export type CustomDomainUpdateBody = { spec: Partial<CustomDomainSpec> };

export const customDomainsKey = () => ['custom-domains'] as const;
export const customDomainKey = (name: string) => ['custom-domain', name] as const;

export function useCustomDomains() {
  return useQuery<CustomDomain[]>({
    queryKey: customDomainsKey(),
    queryFn: async () => unwrapList<CustomDomain>(await api.get('/custom-domains')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateCustomDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CustomDomainCreateBody) => api.post<CustomDomain>('/custom-domains', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: customDomainsKey() }),
  });
}

export function useUpdateCustomDomain(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CustomDomainUpdateBody) =>
      api.patch<CustomDomain>(`/custom-domains/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: customDomainsKey() });
      qc.invalidateQueries({ queryKey: customDomainKey(name) });
    },
  });
}

export function useDeleteCustomDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/custom-domains/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: customDomainsKey() }),
  });
}
