import type { Dataset, DatasetSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  return useQuery<Dataset[]>({
    queryKey: datasetsKey(),
    queryFn: async () => unwrapList<Dataset>(await api.get('/datasets')),
    ...QUERY_DEFAULTS,
  });
}

export function useDataset(name: string | undefined) {
  return useQuery<Dataset>({
    queryKey: datasetKey(name ?? ''),
    queryFn: () => api.get<Dataset>(`/datasets/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
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
