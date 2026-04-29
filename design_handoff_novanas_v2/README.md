# Handoff: NovaNAS V2 — Storage OS Web Console

## Overview

NovaNAS is a self-hosted NAS / homelab operating system. **NovaNAS V2** is the redesigned web console — a desktop-style environment in the browser where an admin manages pools, datasets, snapshots, replication, shares, identity, observability, virtualization, apps/plugins, and system settings.

The design is a full **window-managed desktop OS** that lives inside a single browser tab. The user lands on a wallpapered desktop with a top bar, a centered dock, and floating application windows — not a sidebar/page web app. This is a deliberate choice: NAS admin tasks are inherently multi-tasking (watch a scrub while editing a snapshot policy while skimming logs) and the desktop metaphor lets multiple tools live side by side.

---

## About the Design Files

The files in this bundle are **design references created in HTML/JSX** — visual prototypes showing intended look, layout, and behavior. **They are not production code to ship.**

The task is to **recreate these designs in NovaNAS's existing front-end environment** (or, if no front-end has been chosen yet, in whichever framework fits the codebase — React + TypeScript is the natural fit). Use the existing app's component library, routing, state-management, theming, and accessibility primitives. Do **not** copy the `<script type="text/babel">`-based Babel-in-the-browser setup — that's a prototyping convenience, not production architecture.

What you should lift directly:
- Visual specs (colors, spacing, typography, radii, shadows)
- Layout structure of each app/window
- Component compositions (what's a card vs a table vs a side panel)
- Interaction patterns (tabs, palettes, drawers, dialogs)
- Information density and hierarchy

What you should re-implement in your own stack:
- The window manager
- All data fetching (the prototype uses static mock data)
- All forms (the prototype shows the visual shell, not real validation)
- All animations/transitions (use your codebase's motion library)

---

## Fidelity

**High-fidelity (hifi).** All colors, type sizes, spacing, radii, shadows, and component anatomies are intentional and final. Recreate pixel-perfectly using NovaNAS's existing design system / token set; if no system exists yet, lift the tokens listed in **Design Tokens** below verbatim.

The app currently ships **three theme variants** (Aurora / Graphite / Aether). Aurora is the default and the canonical look. Graphite and Aether are documented but secondary; ship Aurora first.

---

## Backend Conventions (must match exactly)

The prototype's mock data was retrofitted to match the live backend. When wiring real data, follow these conventions — the API rejects deviations.

**Permission scope tokens** — `nova:<domain>:<verb>` form. Verbs are `read`, `write`, `admin`, or `recover`. Authoritative list lives in `internal/auth/rbac.go`. Examples: `nova:storage:read`, `nova:notifications:write`, `nova:plugins:admin`. The Install consent dialog ships a hard-coded description map for the prototype; production should fetch descriptions from the engine's permission summary instead of duplicating them client-side.

**displayCategory enum** — exactly 14 values, validated server-side. Authoritative list lives in `internal/plugins/manifest.go` (`DisplayCategory`). The Aurora category sidebar should be driven by `GET /api/v1/plugins/categories` (returns all 14 + counts), not by deriving categories from the plugin list — the engine emits zero-count entries on purpose so the sidebar shape is stable across marketplace outages.

**Trust model** — cosign ECDSA P-256, marketplace-level. Each marketplace has one pinned PEM stored in `marketplaces.trust_key_pem`; the engine verifies every tarball in that marketplace's index against that key. There is no per-plugin signing metadata. Display the SHA-256 of the PEM as `sha256:<64-hex>` (the engine derives this server-side and returns it on `GET /api/v1/marketplaces`).

**Marketplace fields the API exposes** — `id`, `name`, `indexUrl`, `trustKeyPem`, `trustKeyUrl`, `locked`, `enabled`, `addedBy`, `addedAt`, `updatedAt`. Anything else (rating, downloads, lastSync, plugin counts) must be derived client-side from the merged index — the backend doesn't store stats.

**Pre-install consent** — `GET /api/v1/plugins/index/{name}/manifest?version=…` parses + cosign-verifies the tarball **in-memory** (no disk side-effects until the user confirms install) and returns `{manifest, permissions}` where `permissions` is the structured summary (willCreate / willMount / willOpen / scopes / category). Render this directly in the consent dialog rather than extracting fields from the raw manifest.

**Dependencies** — declared in `manifest.spec.dependencies[]` as `{name, version, source}` where source is `tier-2` or `bundled`, and version is a semver constraint. The engine resolves the install plan and recursively materializes deps; the consent dialog should show the resolved order, not just the direct deps. Uninstall refuses to drop a plugin with installed dependents unless `?cascade=true`.

---

## Information Architecture

The console is organized into **clusters** of apps. Each app opens as its own window. Apps are listed in the dock, the launcher (grid of all apps), and the top-bar `⌘K` palette.

### App inventory

| Cluster | App | Purpose |
|---|---|---|
| **Storage** | Storage Manager | Pools, vdev tree, datasets, snapshots, disks, encryption (tabbed) |
| | Replication | Replication jobs, targets, snapshot schedules, scrub policies |
| | Shares | Unified protocol shares + per-protocol tabs (SMB, NFS, iSCSI, NVMe-oF) |
| **Identity** | Identity | Users, sessions, login history, Kerberos principals |
| **Apps** | Workloads | Helm releases, catalog, k8s events |
| | Virtualization | VMs, templates, VM snapshots |
| | Package Center | Federated marketplaces, install consent flow, installed plugins |
| **Observability** | Alerts | Active alerts, silences, receivers |
| | Logs | Loki-style log stream w/ LogQL input |
| | Audit | Audit log table |
| | Jobs | Background jobs (scrubs, replication runs) |
| | Notifications | Notification center |
| **System** | Network | Interfaces, RDMA devices |
| | System | Overview, updates, SMTP |
| | File Station | File browser (grid + list views) |
| | Terminal | Embedded shell |
| | Control Panel | Catch-all settings hub |

### Shell chrome (always present)

- **Top bar** (38 px) — burger → launcher, brand mark + product name, list of running window tabs, spacer, `⌘K` search field, bell (notifications drawer), user avatar (user menu), clock.
- **Dock** (bottom, centered) — pinned apps with hover-tooltips. Click to open / focus.
- **Desktop widgets** (top-right column) — Storage capacity ring, Pool list w/ usage bars, Activity feed, Resource monitor sparklines. User can hide via Tweaks.
- **Launcher overlay** — full-screen blurred overlay with a 6-col grid of all apps. Opens from burger or `⌘K` "Open app".
- **`⌘K` palette** — center-top modal, 540 px wide, fuzzy-search apps + actions, keyboard nav.
- **Bell drawer** — top-right anchored panel, list of unread notifications.
- **User menu** — small popover anchored to the avatar, signs out / settings / lock.
- **Login screen** — full-bleed wallpaper, centered avatar + password input. Shown when locked.
- **Tweaks panel** — bottom-right floating panel, only visible when host toggles "Tweaks" mode. Lets the user switch theme variant, accent color, density, fonts, wallpaper, widgets visibility.

---

## Theme Variants

Three top-level themes, applied as a class on the OS root:

- **`os--aurora`** — default. Warm-cool dark neutrals (oklch hue 250), blue accent (`oklch(0.78 0.14 220)`), soft gradient wallpaper with aurora glow + faint 32 px grid, refined window chrome with backdrop blur.
- **`os--graphite`** — utilitarian / sysadmin energy. Near-black (oklch 0.11 hue 250), amber accent (`oklch(0.78 0.14 60)`), monospace UI font, sharper 4 px radii, no blur, faint 24 px grid wallpaper, console banner in bottom-left.
- **`os--aether`** — glassy / depth. Violet accent (`oklch(0.74 0.18 305)`), 12 px radii, heavy backdrop blur (32 px) on windows, layered radial-gradient mesh wallpaper with SVG noise overlay.

Each variant overrides only color tokens, blur tokens, radii, and a few wallpaper rules. All component CSS is in `base.css` and uses tokens — variants don't ship their own component CSS.

---

## Screens / Views

Below: every distinct surface, with layout, components, copy, and interactions. Sizes are at the canonical 1440 × 900 viewport — the app should be responsive but the prototype targets that frame.

### 1. Desktop shell

**Root container:** `position: relative; width: 1440; height: 900; overflow: hidden`

Stack of layers (z-index order, back to front):
1. **Wallpaper** — `position: absolute; inset: 0; z-index: 0`. Per-variant background.
2. **Desktop widgets** — `position: absolute; top: 56; right: 16; z-index: 40; width: 320`. Vertical stack, gap 12.
3. **Windows layer** — `position: absolute; inset: 38 0 0 0; z-index: 50`. Hosts all open app windows. `pointer-events: none` on the layer; `pointer-events: auto` on each window.
4. **Dock** — `position: absolute; bottom: 12; left: 50%; transform: translateX(-50%); z-index: 80`.
5. **Top bar** — `position: absolute; top: 0; left: 0; right: 0; height: 38; z-index: 100`.
6. **Bell drawer / user menu** — z-index 8000 when open.
7. **Cmd-K palette / consent modal** — z-index 9000–9500.
8. **Launcher** — z-index 300 (under modals on purpose, so a modal triggered from launcher still works).

### 2. Top bar

Height `38`, padding `0 8`, `display: flex; align-items: center; gap: 8`. Background `var(--bar-bg)` (translucent), backdrop-filter `blur(14px) saturate(140%)`, bottom border `1px solid var(--line)`.

Left → right:
- **Burger button** (`.topbar__menu`) — 28×28, icon, opens launcher.
- **Brand block** (`.topbar__brand`) — 22×22 logo tile (gradient in Aurora/Aether, solid in Graphite) showing **"N"** glyph, then product name **"NovaNAS"** + small muted **"2.4.0"** version.
- **Vertical divider** (1×18, `var(--line)`).
- **Running tasks** — horizontal row of `.tb-task` chips, one per open window. Active window has `.is-on` (background, 2 px accent underline via inset shadow). Click focuses; `×` on hover closes (optional).
- **Spacer** (`flex: 1`).
- **Search** (`.topbar__search`) — 240×26, search icon + placeholder "Search apps, settings, files…" + `⌘K` kbd hint.
- **Bell** (`.topbar__icon`) — 26×26, 6 px red badge dot when unread.
- **Avatar** — 22×22 circle, gradient bg, white mono initials.
- **Clock** — 11 px, `var(--fg-1)`, format `HH:MM` (24 h).

Top bar palette tokens: `--bar-bg`, `--bar-bg-solid` (used for the badge ring), `--bar-blur`.

### 3. Dock

Bottom-centered. Inner `display: flex; gap: 4; padding: 6`, background `var(--dock-bg)`, border `1px solid var(--line-strong)`, radius `var(--r-lg)` (14 px Aurora / 4 px Graphite / 18 px Aether), shadow `0 14px 40px rgba(0,0,0,0.45)`, backdrop-filter blur.

Each `.dock__btn` is 44×44, radius `var(--r-md)`, icon centered. Hover translates `-3px` Y over 120 ms.

Tooltip `.dock__lbl` floats 8 px above on hover (popover bg, 1px line).

### 4. Window

```
.win
  .win__bar      (32 px, drag handle, title text + window buttons)
  .win-tabs      (optional — apps with tab navigation)
  .win__body     (scroll region)
  .win__resize   (14×14 corner handle, bottom-right)
```

- `min-width: 360; min-height: 240`
- `background: var(--win-bg)` (translucent in Aurora/Aether, opaque in Graphite)
- `border: 1px solid var(--line-strong)`, `border-radius: var(--r-md)`
- `backdrop-filter: var(--win-blur)` (none in Graphite, 20 px in Aurora, 32 px in Aether)
- `box-shadow: var(--win-shadow)` (layered inset highlight + drop shadow)

**Title bar (`.win__bar`):** height 32, padding `0 8 0 12`, background `var(--win-bar)`, bottom border. Title left (icon + 11 px label, fg-1, weight 500). Right side `.win__btns`: minimize / maximize / close, each 22×22, hover bg-3, close button hover red.

**Tabs (`.win-tabs`):** when present, sit between bar and body. Padding `0 10`, background `var(--win-tab-bg)`. Each `button.win-tabs button` is `padding: 8 12; font-size: 11; color: var(--fg-3); text-transform: lowercase`. Active tab gets `var(--fg-0)` color and a 2 px accent-color underline `::after` 8 px inset from each edge.

**Body (`.win__body`):** flex 1, overflow auto. Inner padding usually `12` or `14`; specific apps override.

**Drag/resize:** prototype is static — implement drag from `.win__bar` (excluding buttons), resize from `.win__resize` corner.

### 5. Storage Manager (canonical complex app — study this one)

Window default size ≈ **1080 × 660**. Tabs: `pools`, `vdevs`, `datasets`, `snapshots`, `disks`, `encryption`.

**Pools tab (`.cards-grid`)** — `display: grid; grid-template-columns: repeat(2, 1fr); gap: 10; padding: 12`. Each `.pool-card`:
- Background `var(--bg-2)`, border, radius `var(--r-md)`, padding 12, gap 8, flex column.
- Head: pool name (fg-0, weight 500) with 6 px tier dot (`--hot` red / `--warm` amber / `--cold` blue) + `.tier` pill (uppercase 9 px, 0.06em tracking, soft accent bg).
- Meta row: `Healthy · NVMe · rep×2` style (fg-3, 11 px), space-between with state pill on the right.
- Capacity bar (`.bar`, height 5, accent fill).
- Numbers row: used / total / % free.
- IO row (top-bordered): three columns — Read MB/s, Write MB/s, IOPS, each label uppercase 10 px / value mono 12 px.

**Vdevs tab** — pool selector chips at top, then a tree-style listing. Each vdev row: type pill (`mirror-2`, `raidz2-8`), state pill, expandable disk children.

**Datasets tab** — full-width `.tbl` with columns: Name, Pool, Used, Quota, Protocols, Snaps, Encryption, Compression. Click a row → selected (`.is-on`) → opens `.side-detail` to the right (260 px wide, full height of body). Side panel has stacked `.sect` rows: General / Properties / Quota / Snapshot policy / Sharing.

**Snapshots tab** — same `.tbl` pattern, columns: Name, Pool, Size, Age, Schedule, Hold. Toolbar above with `Take snapshot` primary button + filter dropdown.

**Disks tab (`.disks-split`)** — `display: grid; grid-template-columns: 1fr 240px; gap: 14; padding: 12`. Left: enclosures with `.encl-grid` (6-col grid of 1.8:1 slot tiles). Each `.encl-slot` shows slot number top-left, LED dot top-right (green / amber / off / spare-grey), model + size at bottom, `data-state="HEALTHY|DEGRADED|EMPTY|SPARE"`. Empty slots use diagonal stripe pattern. Click selects → right panel `.disk-detail` shows model, serial, SMART summary, temperature, hours, errors.

**Encryption tab** — table of encrypted datasets with status / key format / key location (e.g. `tpm:sealed`) / last rotation. Action buttons `Rotate key`, `Unlock`.

### 6. Replication

Tabs: `jobs`, `targets`, `schedules`, `scrub`.

- **Jobs** — left list of replication jobs (vlist pattern), right detail panel with Source / Target / Schedule / Last run (state pill + duration + bytes) / Throughput sparkline / log tail.
- **Targets** — table: name, host, protocol (ssh+zfs / s3), creds status, port. `Add target` primary button.
- **Schedules** — table: name, datasets (chip list), cron, retention, enabled toggle.
- **Scrub** — table: name, pools, cron, priority, builtin badge.

### 7. Shares

Tabs: `unified`, `smb`, `nfs`, `iscsi`, `nvme-of`.

- **Unified** — table: name, protocols (chip per proto), path, clients, state. The "one share, many protocols" abstraction.
- **SMB** — name, path, users, guest toggle, recycle toggle, vfs modules.
- **NFS** — name, path, clients (CIDR), options (`rw,sync,sec=krb5p,no_subtree_check` style mono), active toggle.
- **iSCSI** — IQN (mono), LUNs, portals (multi-line mono), ACLs, state.
- **NVMe-oF** — NQN (mono), namespaces, ports, hosts, DH-CHAP toggle, state.

### 8. Identity

Tabs: `users`, `sessions`, `login`, `kerberos`.

- **Users** — table: name, role, email, created, last login, MFA pill, status pill. Click → side detail with full profile + reset MFA / disable / delete actions.
- **Sessions** — table: id (mono short), user, IP, UA string (truncated), started, current pill. Revoke button.
- **Login history** — chronological table: timestamp, user, IP, result (success / fail pill), method (`password+totp`, `webauthn`, etc.).
- **Kerberos** — principals table: name (mono full SPN), created, expires, key version, type. `Refresh keytab` action.

### 9. Workloads (Helm releases on embedded k8s)

Tabs: `releases`, `catalog`, `events`.

- **Releases** — left vlist of release names, right detail: chart, version, namespace, pods (e.g. `5/5`), CPU / mem usage bars, status pill (Deployed / Pending / Failed). Action buttons Upgrade / Rollback / Uninstall.
- **Catalog** — `.appcards` grid, each card shows chart icon + name + category + Install button.
- **Events** — chronological list with kind (Normal / Warning), reason, object, message.

### 10. Virtualization

Tabs: `vms`, `templates`, `snapshots`.

- **VMs** (`.app-vm`) — `grid-template-columns: 200px 1fr`. Left `.vm-list` of VMs with status dot (green glow when running). Right `.vm-detail`:
  - **VM screen** (`.vm-screen`) — 16:9 console mock, dark bg, blinking cursor. Stopped VMs render at 0.5 opacity.
  - **Controls row** — Start / Stop / Reset / Console / Snapshot buttons.
  - **Specs grid** — OS, vCPU, RAM, disk, IP, MAC, uptime.
- **Templates** — table: name, OS, vCPU, RAM, disk, source (cloud-image / iso). `Create from template` action.
- **Snapshots** — table: VM/snap-name, parent VM, age, size. Restore / Delete actions.

### 11. Package Center (the most novel surface — read carefully)

Tabs: `discover`, `installed`, `marketplaces`.

NovaNAS supports **federated marketplaces** — multiple plugin sources, each with a trust key. The default is `novanas-official`; users can add `truecharts`, `community`, etc.

- **Discover** — toolbar: source dropdown ("All sources / NovaNAS Official / TrueCharts / …"), category chips (All / AI / Admin / Media / …), search input. Below: `.appcards` grid (3 cols at 720+) of `.mkt-card`:
  - 40×40 mono initials icon tile.
  - Name (weight 500, 13 px), 10 px muted author.
  - Description (11 px, fg-2, line-height 1.45).
  - Foot: trust badge (`.trust-badge--official` = blue / `.trust-badge--community` = purple-grey), version, **Install** button.
- **Install consent dialog** (`.modal-bg` + `.modal`) — when user clicks Install:
  - Modal head with plugin icon + name + author.
  - Body: source + cosign trust-key fingerprint (mono, `sha256:<64-hex>` — the SHA-256 of the marketplace's pinned PEM), then **Permissions list** — each `.perm-row` shows the scope token (mono, e.g. `nova:system:read`) on the left and a human description on the right (e.g. "Read host info — hostname, version, uptime"). Production should fetch both the scope set and the descriptions from `GET /api/v1/plugins/index/{name}/manifest?version=…`, which returns the engine's structured permissions summary.
  - Foot: `Cancel` / `Install` (primary, disabled until checkbox).
- **Installed** — left vlist of installed plugins, right detail: source (with trust badge), version, status (running / stopped), permissions granted, dependencies, last updated. Stop / Restart / Uninstall buttons.
- **Marketplaces** — table: name, URL, trust fingerprint (mono, full), updated. Add / Remove actions.

### 12. Alerts

Tabs: `active`, `silences`, `receivers`.

- **Active** — left list of firing alerts (severity dot + name + age), right detail: severity pill, since timestamp, summary, labels (chip list `instance=disk-13`, `pool=bulk`), runbook link, **Silence** button.
- **Silences** — table: id (mono), matchers (mono chip list), creator, comment, starts, ends. Expire action.
- **Receivers** — table: name, integrations (chips: smtp / webhook / pagerduty / slack).

### 13. Logs

Single pane:
- Top toolbar: LogQL input (mono, default `{job="systemd"} |~ "(?i)error"`), label dropdown chips (job / instance / level / unit / pod / namespace / app), live toggle, time range.
- Body `.log-stream` — mono 11 px lines, each `.log-line` is a 4-col grid: timestamp (gray) / level (colored: info=blue, warn=amber, error=red, debug=gray) / unit / message. Auto-scroll when live.

### 14. Audit, Jobs, Notifications

All three are simple table apps:
- **Audit** — at, actor, action (mono), resource, result, IP.
- **Jobs** — id (mono short), kind (mono), target, state pill (running / done / failed), progress bar + pct, ETA, started, last log line.
- **Notifications** — list of `.notif-item` rows: severity dot, title, time, source, actor. Mark all read button.

### 15. Network

Tabs: `interfaces`, `rdma`.

- **Interfaces** — table: name, type, state, IPv4 (mono w/ CIDR), MAC (mono), MTU, speed, driver.
- **RDMA** — table: name, port, state, speed (`100 Gb/s`), LID, GID (mono).

### 16. System

Tabs: `overview`, `update`, `smtp`.

- **Overview** — KPI strip (`.kpi` cards): hostname, version, uptime, kernel, CPU, RAM, ZFS version. Below: hardware summary card (model, serial, BIOS).
- **Update** — current channel (stable / beta), available version banner with changelog, `Apply update` primary button (disabled when up-to-date).
- **SMTP** — form: host, port, TLS toggle, auth user, auth pass, from-address, reply-to. `Send test` button.

### 17. File Station

Toolbar: path breadcrumb (`/bulk/family-media/Photos`), view toggle (grid / list), upload button.

- **Tree** (`.files-tree`) — left rail, 160 px, lists shares + favorites. Each `.files-tree__item` icon + name; selected gets accent-soft bg.
- **Grid view** (`.files-grid`) — `repeat(auto-fill, minmax(80px, 1fr))`, each item: 48×40 icon tile (folder/image/video colored differently) + 10 px name.
- **List view** (`.files-list`) — 4-col grid: icon, name, size, modified.

### 18. Terminal

Black-ish bg `oklch(0.07 0.01 250)`, mono 11 px, line-height 1.6. `.term-prompt` colored accent, `.term-out` muted green-gray, blinking cursor block.

### 19. Control Panel

`.app-control` — `display: grid; grid-template-columns: repeat(3, 1fr); gap: 10; padding: 14`.

Each `.control-card` has a head row (32×32 accent-soft icon tile + name) and an `.control-card__items` ul of clickable rows that link to other apps (e.g. "Network → Interfaces" launches Network app on that tab).

### 20. Login screen

Full-bleed wallpaper, centered card 320 × 360:
- 64×64 avatar
- Username (large, fg-0)
- Password input
- "Sign in" primary button
- Small footer: hostname (mono) + version

---

## Tweaks Panel

Floating bottom-right panel, only visible when host activates "Tweaks" mode (the prototype communicates with its host via `__edit_mode_*` postMessage; in production, this is just an admin-mode preferences pane).

Sections + controls:
- **Theme**
  - Variant — radio: Aurora / Graphite / Aether
  - Accent — color picker
  - Density — radio: Compact / Default / Spacious
- **Typography**
  - UI font — select: Geist / Inter / IBM Plex Sans / System UI
  - Mono font — select: Geist Mono / JetBrains Mono / IBM Plex Mono / ui-monospace
  - Mono everywhere — toggle (forces UI font to mono)
- **Desktop**
  - Wallpaper — radio: Stars / Aurora / Solid
  - Show widgets — toggle

---

## Interactions & Behavior

- **Window focus** — clicking any window or its top-bar task chip raises it (z-index swap) and marks the chip `.is-on`. The active window's title bar stays the same — focus is communicated only via z-order + the top-bar chip.
- **Window controls** — minimize hides (chip stays in top bar with no `.is-on`); maximize sets window bounds to fit the windows-layer; close removes the window from state.
- **Drag** — pointer-down on `.win__bar` (excluding buttons) → track delta → update window position. Constrain to keep title bar within the windows-layer.
- **Resize** — pointer-down on `.win__resize` → update width/height. Respect `min-width: 360; min-height: 240`.
- **Dock click** — opens a new window if not running; focuses if running. No "minimize to dock" — minimized windows live in the top-bar tabs only.
- **Launcher** — opens with 200 ms fade. Click an item → launcher closes + window opens. Esc / click-outside closes.
- **`⌘K` palette** — Cmd/Ctrl+K from anywhere. Fuzzy filter on apps + recent actions ("Take snapshot", "Open Storage Manager", etc.). ↑/↓ navigates, Enter executes, Esc closes.
- **Bell drawer** — click bell → drawer slides in from top-right. Click outside or bell again to close. `Mark all read` clears badge.
- **User menu** — click avatar → small popover with user identity + Settings / Lock / Sign out.
- **Tabs** — click switches; no animation.
- **Tables** — click row selects (`.is-on`); on selection-required apps (datasets, alerts, jobs, plugins) the side detail panel renders the selected row.
- **Install consent** — checkbox must be checked to enable Install button. The plugin's permission set is the **complete and final** set granted; no incremental prompts later.
- **Login screen** — appears on Lock action or on session expiry. Wallpaper persists; topbar/dock hidden.

### Animations
- Dock buttons: `transform: translateY(-3px)` on hover, 120 ms ease.
- Launcher: 200 ms fade-in.
- Status LED `--err`: 8 px glow.
- Terminal cursor + VM-screen cursor: `blink 1.1s step-end infinite`.
- Hover on table rows / tree items / dock buttons: instant background change (no transition).

---

## State Management

The production app needs:

- **Sessions/auth** — current user, MFA status, lock state.
- **Window manager** — array of `{id, app, x, y, w, h, z, minimized, maximized, props}`. Active window id. Z-order counter.
- **App-level state** — most apps have at least: selected tab, selected row id, search query, filter chips. Side-detail panels are derived from selection.
- **Theme state** — variant, accent, density, ui font, mono font, wallpaper, widgetsVisible. Persist to user prefs.
- **Notifications** — list, unread count.
- **Real-time data** — pool stats, alerts, jobs, logs, sessions all want websocket / SSE updates. Resource Monitor renders sparklines from rolling buffers (`useSeries(40, …)` in the prototype generates 40-sample drift).
- **Async actions** — install plugin, take snapshot, start replication, start VM all return jobs and surface in the Jobs app + a notification.

---

## Design Tokens

All tokens are CSS custom properties applied at the OS root. Variant classes (`os--aurora`, `os--graphite`, `os--aether`) override the color/blur/radius set; `base.css` defines structural tokens that don't change.

### Structural (in `:root`)

```css
--r-xs: 4px;
--r-sm: 6px;
--r-md: 9px;   /* Aurora; Graphite=4px; Aether=12px */
--r-lg: 14px;  /* Aurora; Graphite=6px; Aether=18px */
--r-xl: 20px;
--font-sans: "Geist", "Inter", system-ui, -apple-system, sans-serif;
--font-mono: "Geist Mono", "JetBrains Mono", ui-monospace, Menlo, monospace;
```

### Aurora (default, dark)

```css
--bg-0: oklch(0.16 0.006 250);
--bg-1: oklch(0.19 0.007 250);
--bg-2: oklch(0.22 0.008 250);
--bg-3: oklch(0.27 0.009 250);
--bg-4: oklch(0.32 0.010 250);

--line:        oklch(0.32 0.010 250 / 0.55);
--line-strong: oklch(0.40 0.012 250 / 0.75);

--fg-0: oklch(0.97 0.005 250);  /* primary text */
--fg-1: oklch(0.86 0.006 250);  /* body */
--fg-2: oklch(0.68 0.008 250);  /* secondary */
--fg-3: oklch(0.52 0.009 250);  /* muted */
--fg-4: oklch(0.40 0.010 250);  /* faintest */

--accent:      oklch(0.78 0.14 220);
--accent-soft: oklch(0.78 0.14 220 / 0.18);
--accent-fg:   oklch(0.18 0.02 220);   /* text on accent */

--ok:   oklch(0.78 0.14 150);
--warn: oklch(0.82 0.14 80);
--err:  oklch(0.72 0.17 25);
--info: oklch(0.78 0.12 240);

--bar-bg:      oklch(0.19 0.007 250 / 0.86);
--bar-blur:    blur(14px) saturate(140%);
--input-bg:    oklch(0.16 0.006 250);
--popover-bg:  oklch(0.22 0.008 250 / 0.95);
--launcher-bg: oklch(0.13 0.006 250 / 0.78);
--dock-bg:     oklch(0.22 0.008 250 / 0.78);
--win-bg:      oklch(0.20 0.007 250 / 0.96);
--win-bar:     oklch(0.23 0.008 250);
--win-blur:    blur(20px) saturate(140%);
--win-shadow:
  0 1px 0 oklch(1 0 0 / 0.04) inset,
  0 18px 50px oklch(0 0 0 / 0.55),
  0 4px 12px oklch(0 0 0 / 0.3);
--widget-bg:   oklch(0.20 0.007 250 / 0.78);
```

Wallpaper: `radial-gradient(900px 500px at 8% 12%, oklch(0.45 0.18 220 / 0.45), transparent 60%), radial-gradient(700px 600px at 95% 85%, oklch(0.45 0.16 290 / 0.35), transparent 60%), radial-gradient(600px 400px at 70% 20%, oklch(0.55 0.14 180 / 0.22), transparent 60%), linear-gradient(180deg, oklch(0.13 0.006 250) 0%, oklch(0.10 0.006 250) 100%)` + a 32 px grid overlay at `oklch(1 0 0 / 0.012)`.

### Graphite (alt — sysadmin)

Override only:

```css
--bg-0: oklch(0.11 0.002 250);  /* darker, less chroma */
--accent: oklch(0.78 0.14 60);   /* amber */
--bar-blur: none;
--win-blur: none;
--r-md: 4px; --r-sm: 3px; --r-lg: 6px;
font-family: var(--font-mono);   /* whole UI in mono */
```

Wallpaper: solid `oklch(0.07 0.002 250)` + 24 px grid + bottom-left mono banner `NOVANAS · CONSOLE · 192.168.1.10 · v2.0.0-rc.3`.

Several surfaces also force mono + uppercase tracking on labels: `.win__bar`, `.widget__title`, `.win-tabs button`, `.topbar__name`.

### Aether (alt — glassy)

```css
--bg-0: oklch(0.18 0.012 290);  /* violet hue */
--accent: oklch(0.74 0.18 305);
--line: oklch(1 0 0 / 0.08);
--line-strong: oklch(1 0 0 / 0.16);
--bar-blur: blur(28px) saturate(180%);
--win-blur: blur(32px) saturate(160%);
--r-md: 12px; --r-sm: 8px; --r-lg: 18px;
--win-bg: oklch(0.24 0.014 290 / 0.62);  /* much more transparent */
```

Wallpaper: 4-stop radial mesh + SVG fractal-noise overlay at 0.35 opacity / overlay blend.

### Spacing scale

The prototype uses ad-hoc px values. Recommend formalizing as:

```
4 / 6 / 8 / 10 / 12 / 14 / 16 / 20 / 24 / 32
```

Most cards use 12 padding, gap 8. Most tables use 6×10 cell padding. KPI cards use 8×12.

### Typography scale

| Token | Size | Use |
|---|---|---|
| micro | 9 px | uppercase section eyebrows, kbd hints |
| eyebrow | 10 px | uppercase labels (`KPI__lbl`, `sect__title`, `tier`) — letter-spacing 0.06–0.08em |
| caption | 10 px | grid item names, ring sub-labels |
| label | 11 px | dock tooltips, table cells, side-panel body |
| body | 12 px | default |
| body+ | 13 px | window title text size, mkt-card name |
| h3 | 16 px | KPI value |
| h2 | 22 px | ring center label (mono) |

Body uses font-feature-settings `"ss01", "cv11"` (Geist stylistic alternates).
Mono uses `"tnum" 1` (tabular numerals).

Letter-spacing: body `-0.005em`. Eyebrows / window titles: `+0.04em` to `+0.10em`.

### Shadows

| Token | Use |
|---|---|
| `0 12px 32px rgba(0,0,0,0.4)` | popovers (top-bar bell drawer) |
| `0 14px 40px rgba(0,0,0,0.45)` | dock |
| `var(--win-shadow)` | windows (stacked: inner highlight + 50px drop + 12px tight) |
| `0 16px 40px oklch(0 0 0 / 0.55)` | bell drawer, user menu |
| `0 24px 48px oklch(0 0 0 / 0.6)` | modals |
| `0 30px 60px oklch(0 0 0 / 0.6)` | palette |

### Border radii

| Token | Aurora | Graphite | Aether |
|---|---|---|---|
| `--r-xs` | 4 | 4 | 4 |
| `--r-sm` | 6 | 3 | 8 |
| `--r-md` | 9 | 4 | 12 |
| `--r-lg` | 14 | 6 | 18 |
| `--r-xl` | 20 | 20 | 20 |

### Z-index layers

- 0 wallpaper · 40 widgets · 50 windows · 80 dock · 100 top-bar · 200 top-bar popover · 300 launcher · 8000 bell-drawer / user-menu · 9000 modal · 9500 palette.

---

## Assets

**Fonts** — Geist + Geist Mono are first-pick (Vercel, OFL). Production app should self-host these (or fall back to Inter + JetBrains Mono, both also OFL). Tweaks panel exposes IBM Plex as a third option.

**Icons** — the prototype uses inline SVGs in `src/icons.jsx` (~30 line-icons: pool, dataset, snapshot, disk, encryption, replication, share, smb, nfs, iscsi, nvmeof, network, rdma, alert, log, audit, job, notification, user, session, plugin, marketplace, vm, workload, terminal, file, folder, search, bell, …). They follow a 16×16 / 1.5 stroke / round-cap / round-join style. Use your existing icon set if you have one (Lucide / Phosphor work well at this density); match the stroke weight.

**Imagery** — the only "imagery" is wallpapers, all generated with CSS gradients + SVG noise. No raster assets.

**Brand mark** — letter "N" in a 22×22 rounded tile, gradient fill (Aurora) or solid accent (Graphite), white glyph. Treat as the smallest version of the NovaNAS logo.

---

## Files in this bundle

```
NovaNAS V2.html        — root; loads all scripts, mounts <App>
src/
  app.jsx              — App shell (sets up the 1440×900 frame, mounts <Desktop> + <NovaTweaks>)
  desktop.jsx          — desktop shell: top bar, dock, launcher, palette, widgets, login
  wm.jsx               — window manager (open/close/focus/drag/resize state)
  apps.jsx             — first-pass app components (Storage, AppCenter, FileStation,
                         Virtualization, ControlPanel, ResourceMonitor, Notifications, Terminal)
  apps-core.jsx        — second-pass apps (StorageManager v2, Replication, Network, Shares, Identity)
  apps-installable.jsx — third-pass apps (Workloads, Virt2, PackageCenter, Alerts, Logs,
                         Audit, JobsApp, NotificationCenter, SystemApp, FileStationApp, TerminalApp2)
  data.jsx             — all mock data (POOLS, VDEV_TREE, DATASETS, SNAPSHOTS, SCHEDULES,
                         SCRUB_POLICIES, REPL_TARGETS, REPL_JOBS, ENCRYPTED_DATASETS,
                         NETWORK_INTERFACES, RDMA_DEVICES, APPS, WORKLOADS, VMS, VM_TEMPLATES,
                         VM_SNAPSHOTS, PLUGINS, MARKETPLACE_PLUGINS, MARKETPLACES, USERS,
                         SESSIONS, LOGIN_HISTORY, KRB5_PRINCIPALS, ALERTS, ALERT_SILENCES,
                         ALERT_RECEIVERS, LOG_LABELS, LOG_LINES, AUDIT, JOBS, NOTIFICATIONS,
                         NFS_EXPORTS, SMB_SHARES, ISCSI_TARGETS, NVMEOF_SUBSYSTEMS,
                         PROTOCOL_SHARES, SYSTEM_INFO, SYSTEM_UPDATE, SMTP_CONFIG, ACTIVITY,
                         FILES)
  icons.jsx            — line-icon set used across apps
  tweaks.jsx           — NovaTweaks component (theme/typography/desktop section content)
  tweaks-panel.jsx     — generic TweaksPanel chrome + Tweak* form controls
styles/
  base.css             — all structural / component CSS (1156 lines)
  aurora.css           — Aurora variant tokens + wallpaper
  graphite.css         — Graphite variant tokens + wallpaper + mono overrides
  aether.css           — Aether variant tokens + wallpaper + glass overrides
```

When recreating in production:
1. Start with the **Aurora token set** + `base.css` structural CSS — that's the visual foundation.
2. Build the **window manager + top bar + dock + launcher** as your shell.
3. Build apps in this order of complexity (each unlocks more patterns): **Storage Manager** (tabs / tables / cards / side-detail) → **Package Center** (federated lists + consent modal) → **Logs** (live stream) → **Replication / Identity / Alerts** (variations on the same patterns) → polish apps (System / Network / Shares / Audit / Jobs / Notifications / Control Panel / Terminal / File Station).
4. Add Graphite + Aether as theme switches once Aurora is done. They cost almost nothing on top — just token overrides.

---

## Open questions / things to verify with the team

- **Font licensing** — confirm Geist is acceptable for the brand or pick the substitute up front.
- **Window manager scope** — is multi-window genuinely shipped, or does production launch each app full-frame inside a single content area? The visual language survives either way; the WM is the bigger build.
- **Plugin signing** — backend uses cosign (ECDSA P-256). The prototype renders the SHA-256 of the marketplace's pinned PEM as `sha256:<64-hex>`; the engine stores the PEM in `marketplaces.trust_key_pem` and derives the fingerprint server-side. Trust is established at the marketplace level (per-marketplace key) and propagated to every artifact in that index — there is no per-plugin signing flag in the engine.
- **Density tweak** — Compact / Default / Spacious is wired but the prototype only stubs the spacing changes. Decide concrete spacing scales per density before shipping.
- **Accessibility** — the prototype is mouse-first. Production needs full keyboard nav across the WM (focus traps in modals, Esc to close, arrow keys in lists) and proper ARIA on tabs / tables / palette.
