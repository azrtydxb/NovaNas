import { create } from "zustand";
import { persist } from "zustand/middleware";

export type ThemeVariant = "aurora" | "graphite" | "aether";
export type Density = "compact" | "default" | "spacious";

type ThemeState = {
  variant: ThemeVariant;
  density: Density;
  showWidgets: boolean;
  setVariant: (v: ThemeVariant) => void;
  setDensity: (d: Density) => void;
  setShowWidgets: (b: boolean) => void;
};

export const useTheme = create<ThemeState>()(
  persist(
    (set) => ({
      variant: "aurora",
      density: "default",
      showWidgets: true,
      setVariant: (variant) => set({ variant }),
      setDensity: (density) => set({ density }),
      setShowWidgets: (showWidgets) => set({ showWidgets }),
    }),
    { name: "novanas:theme" }
  )
);
