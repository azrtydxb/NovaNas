import { ThemeBridge } from '@/components/chrome/theme-bridge';
import { JobProgressToaster } from '@/components/common/job-progress-toast';
import { Toaster } from '@/components/common/toaster';
import { ToastProvider, ToastViewport } from '@/components/ui/toast';
import { TooltipProvider } from '@/components/ui/tooltip';
import { i18n } from '@/lib/i18n';
import { createQueryClient } from '@/lib/query-client';
import { I18nProvider } from '@lingui/react';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { useMemo } from 'react';
import { createRouter } from './router';

export function App() {
  const queryClient = useMemo(() => createQueryClient(), []);
  const router = useMemo(() => createRouter(queryClient), [queryClient]);
  return (
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <ThemeBridge />
        <TooltipProvider delayDuration={150}>
          <ToastProvider>
            <RouterProvider router={router} />
            <Toaster />
            <JobProgressToaster />
            <ToastViewport />
          </ToastProvider>
        </TooltipProvider>
      </QueryClientProvider>
    </I18nProvider>
  );
}
