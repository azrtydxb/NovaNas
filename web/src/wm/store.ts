import { create } from "zustand";
import type { AppId, WindowState } from "./types";

type WMState = {
  windows: WindowState[];
  zCounter: number;
  activeId: string | null;
  open: (appId: AppId) => void;
  close: (id: string) => void;
  focus: (id: string) => void;
  toggleMinimize: (id: string) => void;
  toggleMaximize: (id: string) => void;
  move: (id: string, x: number, y: number) => void;
  resize: (id: string, w: number, h: number) => void;
};

let idSeq = 0;

export const useWM = create<WMState>((set, get) => ({
  windows: [],
  zCounter: 1,
  activeId: null,
  open: (appId) => {
    const existing = get().windows.find((w) => w.appId === appId);
    if (existing) {
      get().focus(existing.id);
      if (existing.minimized) {
        set((s) => ({
          windows: s.windows.map((w) =>
            w.id === existing.id ? { ...w, minimized: false } : w
          ),
        }));
      }
      return;
    }
    const id = `w${++idSeq}`;
    const z = get().zCounter + 1;
    const offset = (get().windows.length % 5) * 24;
    set((s) => ({
      windows: [
        ...s.windows,
        {
          id,
          appId,
          x: 80 + offset,
          y: 60 + offset,
          w: 1080,
          h: 660,
          z,
          minimized: false,
          maximized: false,
        },
      ],
      zCounter: z,
      activeId: id,
    }));
  },
  close: (id) =>
    set((s) => ({
      windows: s.windows.filter((w) => w.id !== id),
      activeId: s.activeId === id ? null : s.activeId,
    })),
  focus: (id) => {
    const z = get().zCounter + 1;
    set((s) => ({
      windows: s.windows.map((w) => (w.id === id ? { ...w, z } : w)),
      zCounter: z,
      activeId: id,
    }));
  },
  toggleMinimize: (id) =>
    set((s) => ({
      windows: s.windows.map((w) =>
        w.id === id ? { ...w, minimized: !w.minimized } : w
      ),
    })),
  toggleMaximize: (id) =>
    set((s) => ({
      windows: s.windows.map((w) =>
        w.id === id ? { ...w, maximized: !w.maximized } : w
      ),
    })),
  move: (id, x, y) =>
    set((s) => ({
      windows: s.windows.map((w) => (w.id === id ? { ...w, x, y } : w)),
    })),
  resize: (id, w, h) =>
    set((s) => ({
      windows: s.windows.map((win) => (win.id === id ? { ...win, w, h } : win)),
    })),
}));
