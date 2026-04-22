import { ApiError, api } from '@/lib/api';
import { getUserManager } from '@/lib/auth';
import { type AuthUser, useAuthStore } from '@/stores/auth';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect } from 'react';

export function useAuth() {
  const user = useAuthStore((s) => s.user);
  const status = useAuthStore((s) => s.status);
  const setUser = useAuthStore((s) => s.setUser);
  const setStatus = useAuthStore((s) => s.setStatus);

  const query = useQuery<AuthUser | null>({
    queryKey: ['auth', 'me'],
    queryFn: async () => {
      try {
        return await api.get<AuthUser>('/auth/me');
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null;
        throw err;
      }
    },
    staleTime: 60_000,
    retry: false,
  });

  useEffect(() => {
    if (query.isLoading) setStatus('loading');
    else if (query.isError) setStatus('unauthenticated');
    else if (query.data !== undefined) setUser(query.data);
  }, [query.isLoading, query.isError, query.data, setStatus, setUser]);

  const login = useCallback(async () => {
    await getUserManager().signinRedirect();
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.post('/auth/logout');
    } finally {
      useAuthStore.getState().reset();
      try {
        await getUserManager().signoutRedirect();
      } catch {
        window.location.href = '/login';
      }
    }
  }, []);

  const hasPermission = useCallback((perm: string) => !!user?.permissions.includes(perm), [user]);
  const hasRole = useCallback((role: string) => !!user?.roles.includes(role), [user]);

  return {
    user,
    status,
    isAuthenticated: status === 'authenticated' && !!user,
    isLoading: status === 'loading' || query.isLoading,
    login,
    logout,
    hasPermission,
    hasRole,
    refetch: query.refetch,
  };
}
