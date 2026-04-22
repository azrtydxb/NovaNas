import type { User, UserSpec } from '@novanas/schemas';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type UserCreateBody = { metadata: { name: string }; spec: UserSpec };
export type UserUpdateBody = { spec: Partial<UserSpec> };

export const usersKey = () => ['users'] as const;
export const userKey = (name: string) => ['user', name] as const;

export function useUsers() {
  return useQuery<User[]>({
    queryKey: usersKey(),
    queryFn: async () => unwrapList<User>(await api.get('/users')),
    ...QUERY_DEFAULTS,
  });
}

export function useUser(name: string | undefined) {
  return useQuery<User>({
    queryKey: userKey(name ?? ''),
    queryFn: () => api.get<User>(`/users/${encodeURIComponent(name!)}`),
    enabled: !!name,
    ...QUERY_DEFAULTS,
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UserCreateBody) => api.post<User>('/users', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: usersKey() }),
  });
}

export function useUpdateUser(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UserUpdateBody) =>
      api.patch<User>(`/users/${encodeURIComponent(name)}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usersKey() });
      qc.invalidateQueries({ queryKey: userKey(name) });
    },
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.delete<void>(`/users/${encodeURIComponent(name)}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: usersKey() }),
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ name, password }: { name: string; password: string }) =>
      api.post<void>(`/users/${encodeURIComponent(name)}/reset-password`, { password }),
  });
}
