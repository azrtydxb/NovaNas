import { create } from 'zustand';

export type ToastTone = 'default' | 'success' | 'warning' | 'danger';

export interface ToastItem {
  id: string;
  title: string;
  description?: string;
  tone?: ToastTone;
  duration?: number;
}

interface ToastState {
  toasts: ToastItem[];
  push: (toast: Omit<ToastItem, 'id'> & { id?: string }) => string;
  dismiss: (id: string) => void;
  clear: () => void;
}

let counter = 0;
const nextId = () => `t-${Date.now()}-${++counter}`;

const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  push: (t) => {
    const id = t.id ?? nextId();
    set((s) => ({ toasts: [...s.toasts, { ...t, id }] }));
    return id;
  },
  dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  clear: () => set({ toasts: [] }),
}));

export function useToast() {
  const push = useToastStore((s) => s.push);
  const dismiss = useToastStore((s) => s.dismiss);
  return {
    toast: (t: Omit<ToastItem, 'id'>) => push(t),
    success: (title: string, description?: string) => push({ title, description, tone: 'success' }),
    error: (title: string, description?: string) => push({ title, description, tone: 'danger' }),
    warning: (title: string, description?: string) => push({ title, description, tone: 'warning' }),
    dismiss,
  };
}

export function useToasts() {
  return useToastStore((s) => s.toasts);
}

export function useToastActions() {
  const dismiss = useToastStore((s) => s.dismiss);
  return { dismiss };
}
