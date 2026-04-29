import { create } from "zustand";
import type { User } from "oidc-client-ts";
import { userManager } from "../auth/userManager";

type AuthState = {
  status: "loading" | "anonymous" | "authenticated";
  user: User | null;
  init: () => Promise<void>;
  login: () => Promise<void>;
  logout: () => Promise<void>;
};

export const useAuth = create<AuthState>((set) => ({
  status: "loading",
  user: null,
  init: async () => {
    const user = await userManager.getUser();
    if (user && !user.expired) {
      set({ status: "authenticated", user });
    } else {
      set({ status: "anonymous", user: null });
    }
    userManager.events.addUserLoaded((u) => set({ status: "authenticated", user: u }));
    userManager.events.addUserUnloaded(() => set({ status: "anonymous", user: null }));
    userManager.events.addAccessTokenExpired(() => set({ status: "anonymous", user: null }));
  },
  login: async () => {
    await userManager.signinRedirect();
  },
  logout: async () => {
    await userManager.signoutRedirect();
  },
}));
