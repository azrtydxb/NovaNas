import { create } from 'zustand';

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  username: string;
  roles: string[];
  permissions: string[];
  /**
   * Optional Keycloak-style groups claim. Some installs project roles
   * into groups via the `novanas:roles` mapper rather than the standard
   * `realm_access.roles`, so role-checks should consult both.
   */
  groups?: string[];
}

export type AuthStatus = 'idle' | 'loading' | 'authenticated' | 'unauthenticated';

interface AuthState {
  user: AuthUser | null;
  status: AuthStatus;
  setUser: (user: AuthUser | null) => void;
  setStatus: (status: AuthStatus) => void;
  reset: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  status: 'idle',
  setUser: (user) => set({ user, status: user ? 'authenticated' : 'unauthenticated' }),
  setStatus: (status) => set({ status }),
  reset: () => set({ user: null, status: 'unauthenticated' }),
}));
