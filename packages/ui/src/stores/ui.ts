import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type ThemePreference = 'dark' | 'light' | 'system';
export type ResolvedTheme = 'dark' | 'light';
type Density = 'dense' | 'spacious';

interface UiState {
  sidebarCollapsed: boolean;
  theme: ThemePreference;
  density: Density;
  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setTheme: (t: ThemePreference) => void;
  setDensity: (d: Density) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      theme: 'dark',
      density: 'dense',
      setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setTheme: (theme) => set({ theme }),
      setDensity: (density) => set({ density }),
    }),
    {
      name: 'novanas-ui',
      partialize: (s) => ({
        theme: s.theme,
        density: s.density,
        sidebarCollapsed: s.sidebarCollapsed,
      }),
    }
  )
);

/** Resolve a theme preference down to a concrete value. */
export function resolveTheme(pref: ThemePreference): ResolvedTheme {
  if (pref === 'system') {
    if (typeof window === 'undefined') return 'dark';
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  }
  return pref;
}
