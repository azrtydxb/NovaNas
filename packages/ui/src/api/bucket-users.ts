import type { BucketUser, BucketUserSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type BucketUserCreateBody = {
  metadata: { name: string };
  spec: BucketUserSpec;
};

export const bucketUsersKey = () => ['bucket-users'] as const;
export const bucketUserKey = (name: string) => ['bucket-user', name] as const;

export function useBucketUsers() {
  return useQuery<BucketUser[]>({
    queryKey: bucketUsersKey(),
    queryFn: async () => unwrapList<BucketUser>(await api.get('/bucket-users')),
    ...QUERY_DEFAULTS,
  });
}

export function useBucketUser(name: string | undefined) {
  return useQuery<BucketUser>({
    queryKey: bucketUserKey(name ?? ''),
    queryFn: () => api.get<BucketUser>(`/bucket-users/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateBucketUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BucketUserCreateBody) => api.post<BucketUser>('/bucket-users', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: bucketUsersKey() }),
  });
}

export function useDeleteBucketUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/bucket-users/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: bucketUsersKey() }),
  });
}
