import { useLiveQuery } from '@/hooks/use-live-query';
import type { Vm, VmSpec } from '@novanas/schemas';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type VmCreateBody = {
  metadata: { name: string };
  spec: VmSpec;
};

export type VmUpdateBody = {
  spec: Partial<VmSpec>;
};

export type VmAction = 'start' | 'stop' | 'reset' | 'pause' | 'resume';

export const vmsKey = () => ['vms'] as const;
export const vmKey = (name: string) => ['vm', name] as const;

export function useVms() {
  return useLiveQuery<Vm[]>(vmsKey(), async () => unwrapList<Vm>(await api.get('/vms')), {
    ...QUERY_DEFAULTS,
    staleTime: 60_000,
    wsChannel: 'vm:*',
  });
}

export function useVm(name: string | undefined) {
  return useLiveQuery<Vm>(
    vmKey(name ?? ''),
    () => api.get<Vm>(`/vms/${encodeURIComponent(name!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 60_000,
      enabled: !!name,
      wsChannel: name ? `vm:${name}` : null,
    }
  );
}

export function useCreateVm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VmCreateBody) => api.post<Vm>('/vms', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: vmsKey() }),
  });
}

export function useUpdateVm(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: VmUpdateBody) => api.patch<Vm>(`/vms/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: vmsKey() });
      qc.invalidateQueries({ queryKey: vmKey(name) });
    },
  });
}

export function useDeleteVm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, deleteDisks }: { name: string; deleteDisks?: boolean }) =>
      api.delete<void>(`/vms/${encodeURIComponent(name)}`, {
        searchParams: deleteDisks ? { deleteDisks: true } : undefined,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: vmsKey() }),
  });
}

export function useVmAction(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (action: VmAction) => api.post<Vm>(`/vms/${encodeURIComponent(name)}/${action}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: vmsKey() });
      qc.invalidateQueries({ queryKey: vmKey(name) });
    },
  });
}
