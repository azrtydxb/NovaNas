# @novanas/ui

React 19 + Vite single-page application for the NovaNas web console.

## Stack

- React 19, Vite 5
- TanStack Router (file-based) + Query + Table
- Shadcn-pattern UI primitives on top of Radix + Tailwind
- Zustand (light state), `react-hook-form` + Zod resolvers
- Recharts (inline widgets) + ECharts (heavy detail views)
- `oidc-client-ts` → Keycloak (code + PKCE)
- Lingui (i18n scaffolding)
- Vitest + Testing Library + Playwright

## Running locally

From the repo root:

```bash
pnpm install
pnpm --filter @novanas/ui dev
```

The dev server listens on http://localhost:5173 and proxies `/api` and `/ws`
to `http://localhost:8080` (the `@novanas/api` dev server).

## Environment variables

Create `packages/ui/.env.local` (gitignored) for local overrides:

| Variable                    | Default                             | Purpose                              |
| --------------------------- | ----------------------------------- | ------------------------------------ |
| `VITE_API_BASE`             | `/api/v1`                           | Base path of the NovaNas API         |
| `VITE_OIDC_ISSUER`          | `/auth/realms/novanas`              | Keycloak realm URL                   |
| `VITE_OIDC_CLIENT_ID`       | `novanas-ui`                        | Keycloak client id                   |
| `VITE_OIDC_REDIRECT_URI`    | `${origin}/auth/callback`           | OIDC redirect URL                    |
| `VITE_OIDC_POST_LOGOUT_URI` | `${origin}/login`                   | Post-logout redirect                 |
| `VITE_OIDC_SCOPE`           | `openid profile email`              | OAuth scopes                         |

## Scripts

| Command                | Purpose                                     |
| ---------------------- | ------------------------------------------- |
| `pnpm dev`             | Vite dev server with HMR                    |
| `pnpm build`           | Production build to `dist/`                 |
| `pnpm preview`         | Preview the production build                |
| `pnpm typecheck`       | `tsc --noEmit`                              |
| `pnpm lint`            | Biome check on `src`                        |
| `pnpm test`            | Vitest unit suite                           |
| `pnpm test:e2e`        | Playwright smoke suite                      |

## Directory map

```
src/
  app.tsx           root providers (Query, Tooltip, Toast, Router)
  main.tsx          React 19 root mount
  router.tsx        TanStack Router wiring
  routes/           file-based routes (generated tree → routeTree.gen.ts)
  components/
    chrome/         app shell: Topbar, Sidebar, Brand
    ui/             Shadcn-pattern primitives
    common/         DataTable, EmptyState, HealthPill, Sparkline, Stat …
    charts/         Recharts / ECharts wrappers
  hooks/            use-auth, use-api, use-ws
  lib/              api, ws, auth, query-client, cn, format
  stores/           Zustand stores (auth, ui)
  styles/           globals.css (Tailwind base) + tokens.css (CSS vars)
```

## Auth flow

1. Unauthenticated visitors are bounced to `/login`.
2. The "Sign in with Keycloak" button starts an OIDC code + PKCE redirect.
3. Keycloak redirects to `/auth/callback`, which exchanges the code with
   `POST /api/v1/auth/callback`. The API sets the session cookie and we
   redirect to `/dashboard`.
4. `useAuth()` exposes `user`, `isAuthenticated`, `logout()` and
   permission helpers; the `_auth` route guard enforces authentication
   for all authenticated pages.
