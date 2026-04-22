import type { UpdatePolicy, UpdatePolicySpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api } from './client';

export const updatePolicyKey = () => ['update-policy'] as const;

export function useUpdatePolicy() {
  return useQuery<UpdatePolicy>({
    queryKey: updatePolicyKey(),
    queryFn: () => api.get<UpdatePolicy>('/update-policy'),
    ...QUERY_DEFAULTS,
  });
}

export function useSaveUpdatePolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (spec: Partial<UpdatePolicySpec>) =>
      api.patch<UpdatePolicy>('/update-policy', { spec }),
    onSuccess: () => qc.invalidateQueries({ queryKey: updatePolicyKey() }),
  });
}

export function useCheckForUpdates() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<void>('/update-policy/check', {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: updatePolicyKey() }),
  });
}

export interface UpdateHistoryEntry {
  id: string;
  version: string;
  appliedAt: string;
  status: 'succeeded' | 'failed' | 'rolled-back';
  notes?: string;
}

export function useUpdateHistory() {
  return useQuery<UpdateHistoryEntry[]>({
    queryKey: ['update-history'],
    queryFn: async () => {
      const res = await api.get<{ items?: UpdateHistoryEntry[] } | UpdateHistoryEntry[]>(
        '/update-policy/history'
      );
      return Array.isArray(res) ? res : (res?.items ?? []);
    },
    ...QUERY_DEFAULTS,
  });
}
