import { ApiError, api } from '@/lib/api';
import { startLogin, startLogout } from '@/lib/auth';
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
    await startLogin();
  }, []);

  const logout = useCallback(async () => {
    useAuthStore.getState().reset();
    await startLogout();
  }, []);

  const hasPermission = useCallback((perm: string) => !!user?.permissions?.includes(perm), [user]);
  const hasRole = useCallback((role: string) => !!user?.roles?.includes(role), [user]);
  /**
   * True when the current user has a role that permits mutations.
   * Viewers (and unauthenticated users) cannot mutate. During scaffolding
   * with no user yet, we optimistically return true so the admin-view works
   * before the API session is established.
   */
  const canMutate = useCallback((): boolean => {
    if (!user) return true;
    const roles = user.roles ?? [];
    if (roles.length === 0) return true;
    if (roles.includes('viewer') && !roles.includes('admin') && !roles.includes('user')) {
      return false;
    }
    return roles.includes('admin') || roles.includes('user');
  }, [user]);

  return {
    user,
    status,
    isAuthenticated: status === 'authenticated' && !!user,
    isLoading: status === 'loading' || query.isLoading,
    login,
    logout,
    hasPermission,
    hasRole,
    canMutate,
    refetch: query.refetch,
  };
}
