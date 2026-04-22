import { useLiveQuery } from '@/hooks/use-live-query';
import type { AppInstance, AppInstanceSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type AppInstanceCreateBody = {
  metadata: { name: string };
  spec: AppInstanceSpec;
};

export type AppInstanceUpdateBody = {
  spec: Partial<AppInstanceSpec>;
};

export const appInstancesKey = () => ['app-instances'] as const;
export const appInstanceKey = (name: string) => ['app-instance', name] as const;

export function useAppInstances() {
  return useLiveQuery<AppInstance[]>(
    appInstancesKey(),
    async () => unwrapList<AppInstance>(await api.get('/apps')),
    { ...QUERY_DEFAULTS, staleTime: 60_000, wsChannel: 'appinstance:*' }
  );
}

export function useAppInstance(name: string | undefined) {
  return useLiveQuery<AppInstance>(
    appInstanceKey(name ?? ''),
    () => api.get<AppInstance>(`/apps/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `appinstance:${name}` : null,
    }
  );
}

export function useCreateAppInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AppInstanceCreateBody) => api.post<AppInstance>('/apps', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: appInstancesKey() }),
  });
}

export function useUpdateAppInstance(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AppInstanceUpdateBody) =>
      api.patch<AppInstance>(`/apps/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: appInstancesKey() });
      qc.invalidateQueries({ queryKey: appInstanceKey(name) });
    },
  });
}

export function useDeleteAppInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, deleteData }: { name: string; deleteData?: boolean }) =>
      api.delete<void>(`/apps/${encodeURIComponent(name)}`, {
        searchParams: deleteData ? { deleteData: true } : undefined,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: appInstancesKey() }),
  });
}

export function useAppInstanceAction(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (action: 'start' | 'stop' | 'update') =>
      api.post<AppInstance>(`/apps/${encodeURIComponent(name)}/${action}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: appInstancesKey() });
      qc.invalidateQueries({ queryKey: appInstanceKey(name) });
    },
  });
}
