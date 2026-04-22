import type { Bucket, BucketSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type BucketCreateBody = {
  metadata: { name: string };
  spec: BucketSpec;
};

export type BucketUpdateBody = {
  spec: Partial<BucketSpec>;
};

export const bucketsKey = () => ['buckets'] as const;
export const bucketKey = (name: string) => ['bucket', name] as const;

export function useBuckets() {
  return useQuery<Bucket[]>({
    queryKey: bucketsKey(),
    queryFn: async () => unwrapList<Bucket>(await api.get('/buckets')),
    ...QUERY_DEFAULTS,
  });
}

export function useBucket(name: string | undefined) {
  return useQuery<Bucket>({
    queryKey: bucketKey(name ?? ''),
    queryFn: () => api.get<Bucket>(`/buckets/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateBucket() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BucketCreateBody) => api.post<Bucket>('/buckets', body),
    onSuccess: (b) => {
      qc.invalidateQueries({ queryKey: bucketsKey() });
      if (b?.metadata?.name) qc.setQueryData(bucketKey(b.metadata.name), b);
    },
  });
}

export function useUpdateBucket(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BucketUpdateBody) =>
      api.patch<Bucket>(`/buckets/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: bucketsKey() });
      qc.invalidateQueries({ queryKey: bucketKey(name) });
    },
  });
}

export function useDeleteBucket() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/buckets/${encodeURIComponent(name)}`),
    onSuccess: (_d, name) => {
      qc.invalidateQueries({ queryKey: bucketsKey() });
      qc.removeQueries({ queryKey: bucketKey(name) });
    },
  });
}
