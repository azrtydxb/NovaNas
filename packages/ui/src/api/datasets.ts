import { useLiveQuery } from '@/hooks/use-live-query';
import type { Dataset, DatasetSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type DatasetCreateBody = {
  metadata: { name: string };
  spec: DatasetSpec;
};

export type DatasetUpdateBody = {
  spec: Partial<DatasetSpec>;
};

export const datasetsKey = () => ['datasets'] as const;
export const datasetKey = (name: string) => ['dataset', name] as const;

export function useDatasets() {
  return useLiveQuery<Dataset[]>(
    datasetsKey(),
    async () => unwrapList<Dataset>(await api.get('/datasets')),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'dataset:*' }
  );
}

export function useDataset(name: string | undefined) {
  return useLiveQuery<Dataset>(
    datasetKey(name ?? ''),
    () => api.get<Dataset>(`/datasets/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `dataset:${name}` : null,
    }
  );
}

export function useCreateDataset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DatasetCreateBody) => api.post<Dataset>('/datasets', body),
    onSuccess: (ds) => {
      qc.invalidateQueries({ queryKey: datasetsKey() });
      if (ds?.metadata?.name) qc.setQueryData(datasetKey(ds.metadata.name), ds);
    },
  });
}

export function useUpdateDataset(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DatasetUpdateBody) =>
      api.patch<Dataset>(`/datasets/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: datasetsKey() });
      qc.invalidateQueries({ queryKey: datasetKey(name) });
    },
  });
}

export function useDeleteDataset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/datasets/${encodeURIComponent(name)}`),
    onSuccess: (_d, name) => {
      qc.invalidateQueries({ queryKey: datasetsKey() });
      qc.removeQueries({ queryKey: datasetKey(name) });
    },
  });
}
