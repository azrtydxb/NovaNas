import type { Vm, VmSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  return useQuery<Vm[]>({
    queryKey: vmsKey(),
    queryFn: async () => unwrapList<Vm>(await api.get('/vms')),
    ...QUERY_DEFAULTS,
  });
}

export function useVm(name: string | undefined) {
  return useQuery<Vm>({
    queryKey: vmKey(name ?? ''),
    queryFn: () => api.get<Vm>(`/vms/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
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
