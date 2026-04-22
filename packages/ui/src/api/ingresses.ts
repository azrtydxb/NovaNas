import type { Ingress, IngressSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type IngressCreateBody = { metadata: { name: string }; spec: IngressSpec };
export type IngressUpdateBody = { spec: Partial<IngressSpec> };

export const ingressesKey = () => ['ingresses'] as const;
export const ingressKey = (name: string) => ['ingress', name] as const;

export function useIngresses() {
  return useQuery<Ingress[]>({
    queryKey: ingressesKey(),
    queryFn: async () => unwrapList<Ingress>(await api.get('/ingresses')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateIngress() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: IngressCreateBody) => api.post<Ingress>('/ingresses', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ingressesKey() }),
  });
}

export function useUpdateIngress(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: IngressUpdateBody) =>
      api.patch<Ingress>(`/ingresses/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ingressesKey() });
      qc.invalidateQueries({ queryKey: ingressKey(name) });
    },
  });
}

export function useDeleteIngress() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/ingresses/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ingressesKey() }),
  });
}
