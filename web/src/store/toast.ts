import { create } from "zustand";

export type ToastKind = "success" | "error" | "info";

export type Toast = {
  id: number;
  kind: ToastKind;
  title: string;
  body?: string;
};

type ToastState = {
  toasts: Toast[];
  push: (t: Omit<Toast, "id">) => number;
  dismiss: (id: number) => void;
};

let seq = 0;

export const useToasts = create<ToastState>((set) => ({
  toasts: [],
  push: (t) => {
    const id = ++seq;
    set((s) => ({ toasts: [...s.toasts, { ...t, id }] }));
    setTimeout(() => {
      set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) }));
    }, t.kind === "error" ? 8000 : 4000);
    return id;
  },
  dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) })),
}));

export function toastError(title: string, err: unknown) {
  const body = err instanceof Error ? err.message : String(err);
  useToasts.getState().push({ kind: "error", title, body });
}

export function toastSuccess(title: string, body?: string) {
  useToasts.getState().push({ kind: "success", title, body });
}

export function toastInfo(title: string, body?: string) {
  useToasts.getState().push({ kind: "info", title, body });
}
