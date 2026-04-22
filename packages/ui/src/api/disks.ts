import type { Disk, DiskSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type DiskUpdateBody = {
  spec: Partial<DiskSpec>;
};

export const disksKey = () => ['disks'] as const;
export const diskKey = (wwn: string) => ['disk', wwn] as const;

export function useDisks() {
  return useQuery<Disk[]>({
    queryKey: disksKey(),
    queryFn: async () => unwrapList<Disk>(await api.get('/disks')),
    ...QUERY_DEFAULTS,
  });
}

/**
 * Status watcher: polls more aggressively so SMART/state changes surface quickly.
 */
export function useDisksLive() {
  return useQuery<Disk[]>({
    queryKey: [...disksKey(), 'live'],
    queryFn: async () => unwrapList<Disk>(await api.get('/disks')),
    ...QUERY_DEFAULTS,
    refetchInterval: 15_000,
    staleTime: 5_000,
  });
}

export function useDisk(wwn: string | undefined) {
  return useQuery<Disk>({
    queryKey: diskKey(wwn ?? ''),
    queryFn: () => api.get<Disk>(`/disks/${encodeURIComponent(wwn!)}`),
    enabled: !!wwn,
    ...QUERY_DEFAULTS,
  });
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
