import { useLiveQuery } from '@/hooks/use-live-query';
import type { Snapshot, SnapshotSpec, VolumeSourceRef } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type SnapshotCreateBody = {
  metadata: { name: string };
  spec: SnapshotSpec;
};

export const snapshotsKey = () => ['snapshots'] as const;
export const snapshotsBySourceKey = (source?: VolumeSourceRef) =>
  ['snapshots', source?.kind ?? 'all', source?.name ?? 'all'] as const;
export const snapshotKey = (name: string) => ['snapshot', name] as const;

function sourceSearchParams(source?: VolumeSourceRef) {
  if (!source) return undefined;
  return {
    sourceKind: source.kind,
    sourceName: source.name,
    ...(source.namespace ? { sourceNamespace: source.namespace } : {}),
  };
}

export function useSnapshots(source?: VolumeSourceRef) {
  return useLiveQuery<Snapshot[]>(
    snapshotsBySourceKey(source),
    async () =>
      unwrapList<Snapshot>(
        await api.get('/snapshots', { searchParams: sourceSearchParams(source) })
      ),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'snapshot:*' }
  );
}

export function useSnapshot(name: string | undefined) {
  return useLiveQuery<Snapshot>(
    snapshotKey(name ?? ''),
    () => api.get<Snapshot>(`/snapshots/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `snapshot:${name}` : null,
    }
  );
}

export function useCreateSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: SnapshotCreateBody) => api.post<Snapshot>('/snapshots', body),
    onSuccess: (snap) => {
      qc.invalidateQueries({ queryKey: snapshotsKey() });
      if (snap?.metadata?.name) qc.setQueryData(snapshotKey(snap.metadata.name), snap);
    },
  });
}

export function useUpdateSnapshot(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { spec: Partial<SnapshotSpec> }) =>
      api.patch<Snapshot>(`/snapshots/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: snapshotsKey() });
      qc.invalidateQueries({ queryKey: snapshotKey(name) });
    },
  });
}

export function useDeleteSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/snapshots/${encodeURIComponent(name)}`),
    onSuccess: (_d, name) => {
      qc.invalidateQueries({ queryKey: snapshotsKey() });
      qc.removeQueries({ queryKey: snapshotKey(name) });
    },
  });
}
