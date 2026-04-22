import type { ObjectStore, ObjectStoreSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type ObjectStoreCreateBody = {
  metadata: { name: string };
  spec: ObjectStoreSpec;
};

export const objectStoresKey = () => ['object-stores'] as const;
export const objectStoreKey = (name: string) => ['object-store', name] as const;

export function useObjectStores() {
  return useQuery<ObjectStore[]>({
    queryKey: objectStoresKey(),
    queryFn: async () => unwrapList<ObjectStore>(await api.get('/object-stores')),
    ...QUERY_DEFAULTS,
  });
}

export function useObjectStore(name: string | undefined) {
  return useQuery<ObjectStore>({
    queryKey: objectStoreKey(name ?? ''),
    queryFn: () => api.get<ObjectStore>(`/object-stores/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateObjectStore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ObjectStoreCreateBody) => api.post<ObjectStore>('/object-stores', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: objectStoresKey() }),
  });
}

export function useDeleteObjectStore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/object-stores/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: objectStoresKey() }),
  });
}
