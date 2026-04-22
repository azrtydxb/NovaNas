import type { Certificate, CertificateSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type CertificateCreateBody = { metadata: { name: string }; spec: CertificateSpec };

export const certificatesKey = () => ['certificates'] as const;
export const certificateKey = (name: string) => ['certificate', name] as const;

export function useCertificates() {
  return useQuery<Certificate[]>({
    queryKey: certificatesKey(),
    queryFn: async () => unwrapList<Certificate>(await api.get('/certificates')),
    ...QUERY_DEFAULTS,
  });
}

export function useCreateCertificate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CertificateCreateBody) => api.post<Certificate>('/certificates', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: certificatesKey() }),
  });
}

export function useRenewCertificate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api.post<Certificate>(`/certificates/${encodeURIComponent(name)}/renew`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: certificatesKey() }),
  });
}

export function useDeleteCertificate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/certificates/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: certificatesKey() }),
  });
}
