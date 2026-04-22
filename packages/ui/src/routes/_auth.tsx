import { createFileRoute, redirect } from '@tanstack/react-router';
import { Chrome } from '@/components/chrome/chrome';
import { api, ApiError } from '@/lib/api';
import { useAuthStore, type AuthUser } from '@/stores/auth';

export const Route = createFileRoute('/_auth')({
  beforeLoad: async () => {
    const { status, user } = useAuthStore.getState();
    if (status === 'authenticated' && user) return;
    try {
      const me = await api.get<AuthUser>('/auth/me');
      useAuthStore.getState().setUser(me);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: '/login' });
      }
      // Let other errors surface in the error boundary.
      throw err;
    }
  },
  component: Chrome,
});
