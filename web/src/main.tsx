import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider, MutationCache } from "@tanstack/react-query";
import { App } from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Toasts } from "./components/Toasts";
import { toastError } from "./store/toast";

import "./styles/base.css";
import "./styles/aurora.css";
import "./styles/app.css";

// Surface every mutation error as a toast so buttons can't fail
// silently. Components can still attach an `onError` for inline UI;
// this is the safety net.
const qc = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, refetchOnWindowFocus: false, retry: 1 },
  },
  mutationCache: new MutationCache({
    onError: (err, _vars, _ctx, mutation) => {
      const meta = mutation.meta as { silent?: boolean; label?: string } | undefined;
      if (meta?.silent) return;
      toastError(meta?.label ?? "Action failed", err);
    },
  }),
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <QueryClientProvider client={qc}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
        <Toasts />
      </QueryClientProvider>
    </ErrorBoundary>
  </StrictMode>
);
