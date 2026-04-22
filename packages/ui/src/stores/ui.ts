import { create } from 'zustand';

type Theme = 'dark' | 'light';
type Density = 'dense' | 'spacious';

interface UiState {
  sidebarCollapsed: boolean;
  theme: Theme;
  density: Density;
  setSidebarCollapsed: (v: boolean) => void;
  toggleSidebar: () => void;
  setTheme: (t: Theme) => void;
  setDensity: (d: Density) => void;
}

export const useUiStore = create<UiState>((set) => ({
  sidebarCollapsed: false,
  theme: 'dark',
  density: 'dense',
  setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  setTheme: (theme) => set({ theme }),
  setDensity: (density) => set({ density }),
}));
