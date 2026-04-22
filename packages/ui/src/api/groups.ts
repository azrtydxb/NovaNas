import type { Group, GroupSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type GroupCreateBody = { metadata: { name: string }; spec: GroupSpec };
export type GroupUpdateBody = { spec: Partial<GroupSpec> };

export const groupsKey = () => ['groups'] as const;
export const groupKey = (name: string) => ['group', name] as const;

export function useGroups() {
  return useQuery<Group[]>({
    queryKey: groupsKey(),
    queryFn: async () => unwrapList<Group>(await api.get('/groups')),
    ...QUERY_DEFAULTS,
  });
}

export function useGroup(name: string | undefined) {
  return useQuery<Group>({
    queryKey: groupKey(name ?? ''),
    queryFn: () => api.get<Group>(`/groups/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateGroup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: GroupCreateBody) => api.post<Group>('/groups', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey() }),
  });
}

export function useUpdateGroup(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: GroupUpdateBody) =>
      api.patch<Group>(`/groups/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: groupsKey() });
      qc.invalidateQueries({ queryKey: groupKey(name) });
    },
  });
}

export function useDeleteGroup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/groups/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey() }),
  });
}
