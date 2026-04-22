import { useLiveQuery } from '@/hooks/use-live-query';
import type { Disk, DiskSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type DiskUpdateBody = {
  spec: Partial<DiskSpec>;
};

export const disksKey = () => ['disks'] as const;
export const diskKey = (wwn: string) => ['disk', wwn] as const;

export function useDisks() {
  return useLiveQuery<Disk[]>(disksKey(), async () => unwrapList<Disk>(await api.get('/disks')), {
    ...QUERY_DEFAULTS,
    staleTime: 60_000,
    wsChannel: 'disk:*',
  });
}

/**
 * Status watcher: WS-driven invalidation surfaces SMART/state changes
 * promptly without aggressive polling.
 */
export function useDisksLive() {
  return useLiveQuery<Disk[]>(
    [...disksKey(), 'live'],
    async () => unwrapList<Disk>(await api.get('/disks')),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'disk:*' }
  );
}

export function useDisk(wwn: string | undefined) {
  return useLiveQuery<Disk>(
    diskKey(wwn ?? ''),
    () => api.get<Disk>(`/disks/${encodeURIComponent(wwn!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!wwn,
      wsChannel: wwn ? `disk:${wwn}` : null,
    }
  );
}

export function useUpdateDisk(wwn: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DiskUpdateBody) =>
      api.patch<Disk>(`/disks/${encodeURIComponent(wwn)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: disksKey() });
      qc.invalidateQueries({ queryKey: diskKey(wwn) });
    },
  });
}
