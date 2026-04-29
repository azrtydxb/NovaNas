# NovaNAS Web Console (V2)

React + TypeScript implementation of the NovaNAS V2 design (`/design_handoff_novanas_v2/`).
Runs against a live `nova-api` instance via OIDC + bearer-token auth.

## Stack

- Vite + React 19 + TypeScript
- TanStack Query (server state) + Zustand (WM, theme, auth)
- React Router (deep-linkable apps)
- oidc-client-ts (auth-code + PKCE)
- CSS tokens ported from the design (Aurora variant first; Graphite/Aether to follow)

## Layout

```
src/
  api/         REST client + per-domain query helpers
  auth/        OIDC user-manager, login, callback
  store/       Zustand stores (auth, theme, WM lives in wm/store)
  wm/          Window manager: registry, store, <Window>
  desktop/     TopBar, Dock, Launcher, ⌘K Palette, <Desktop>
  apps/        One folder per application
  components/  Reusable primitives (added as needed)
  styles/      base.css + aurora.css from the design + app.css for our deltas
```

## Running locally

```bash
cd web
npm install
npm run dev
```

The Vite dev server listens on `127.0.0.1:5173` and proxies `/api` to
`https://192.168.10.204:8444` (the dev box). The first time you sign in,
your browser will prompt to trust the Keycloak (`:8443`) self-signed
cert — accept once.

OIDC redirect URIs are configured on the existing `nova-api` Keycloak
client (`http://localhost:5173/*` + `http://127.0.0.1:5173/*`).

## Build

```bash
npm run build   # tsc -b && vite build → dist/
```

The production build expects to be reverse-proxied through nova-api on
the same origin, so `VITE_API_BASE=` (empty) lets API calls go to the
serving origin.

## Phase status

- [x] **1.1** Foundation (this commit): scaffold, auth, WM, top bar,
      dock, launcher, ⌘K, base CSS. Package Center: Discover (with
      live category filter) + Marketplaces table — both wired to the
      live API.
- [ ] **1.2** Package Center: Install consent dialog (preview + perms),
      dependency tree, Installed plugins detail view, Add-marketplace.
- [ ] **2** Storage Manager, Identity, Replication, Shares.
- [ ] **3** Alerts, Logs, Audit, Jobs, Notifications, Network, System,
      SMTP.
- [ ] **4** File Station, Terminal, VMs, Workloads, Control Panel;
      Graphite + Aether themes.
- [ ] **5** Density, a11y, production deploy via nova-api static
      server.
