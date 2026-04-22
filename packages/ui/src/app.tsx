import { ThemeBridge } from '@/components/chrome/theme-bridge';
import { Toaster } from '@/components/common/toaster';
import { ToastProvider, ToastViewport } from '@/components/ui/toast';
import { TooltipProvider } from '@/components/ui/tooltip';
import { createQueryClient } from '@/lib/query-client';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { useMemo } from 'react';
import { createRouter } from './router';

export function App() {
  const queryClient = useMemo(() => createQueryClient(), []);
  const router = useMemo(() => createRouter(queryClient), [queryClient]);
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeBridge />
      <TooltipProvider delayDuration={150}>
        <ToastProvider>
          <RouterProvider router={router} />
          <Toaster />
          <ToastViewport />
        </ToastProvider>
      </TooltipProvider>
    </QueryClientProvider>
  );
}
