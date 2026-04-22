import { useLiveQuery } from '@/hooks/use-live-query';
import type { Share, ShareSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type ShareCreateBody = {
  metadata: { name: string };
  spec: ShareSpec;
};

export type ShareUpdateBody = {
  spec: Partial<ShareSpec>;
};

export const sharesKey = () => ['shares'] as const;
export const shareKey = (name: string) => ['share', name] as const;

export function useShares() {
  return useLiveQuery<Share[]>(
    sharesKey(),
    async () => unwrapList<Share>(await api.get('/shares')),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'share:*' }
  );
}

export function useShare(name: string | undefined) {
  return useLiveQuery<Share>(
    shareKey(name ?? ''),
    () => api.get<Share>(`/shares/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `share:${name}` : null,
    }
  );
}

export function useCreateShare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ShareCreateBody) => api.post<Share>('/shares', body),
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: sharesKey() });
      if (s?.metadata?.name) qc.setQueryData(shareKey(s.metadata.name), s);
    },
  });
}

export function useUpdateShare(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ShareUpdateBody) =>
      api.patch<Share>(`/shares/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sharesKey() });
      qc.invalidateQueries({ queryKey: shareKey(name) });
    },
  });
}

export function useDeleteShare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/shares/${encodeURIComponent(name)}`),
    onSuccess: (_d, name) => {
      qc.invalidateQueries({ queryKey: sharesKey() });
      qc.removeQueries({ queryKey: shareKey(name) });
    },
  });
}
