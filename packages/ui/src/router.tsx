import type { QueryClient } from '@tanstack/react-query';
import { createRouter as createTanstackRouter } from '@tanstack/react-router';
import { routeTree } from './routeTree.gen';

export interface RouterContext {
  queryClient: QueryClient;
}

export function createRouter(queryClient: QueryClient) {
  return createTanstackRouter({
    routeTree,
    defaultPreload: 'intent',
    context: { queryClient },
  });
}

export type AppRouter = ReturnType<typeof createRouter>;

declare module '@tanstack/react-router' {
  interface Register {
    router: AppRouter;
  }
}
