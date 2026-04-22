/**
 * Shared Playwright selectors. Prefer role/text locators; fall back to
 * data-testid for dynamic widgets. Keep this file in sync with the UI
 * component library.
 */

export const selectors = {
  // Global chrome
  appShell: '[data-testid="app-shell"]',
  navLink: (name: string) => `[data-testid="nav-${name}"]`,
  userMenu: '[data-testid="user-menu"]',
  logout: '[data-testid="logout"]',

  // Login (OIDC)
  loginButton: '[data-testid="login-button"]',
  oidcUsername: 'input[name="username"]',
  oidcPassword: 'input[name="password"]',
  oidcSubmit: 'button[type="submit"]',

  // Dashboard
  healthPill: '[data-testid="health-pill"]',
  capacityCard: '[data-testid="capacity-card"]',
  activityFeed: '[data-testid="activity-feed"]',

  // Pools
  poolList: '[data-testid="pool-list"]',
  poolRow: (name: string) => `[data-testid="pool-row-${name}"]`,
  createPoolButton: '[data-testid="create-pool"]',

  // Datasets
  datasetList: '[data-testid="dataset-list"]',
  datasetRow: (name: string) => `[data-testid="dataset-row-${name}"]`,
  createDatasetButton: '[data-testid="create-dataset"]',
  protectionSelect: '[data-testid="protection-select"]',

  // Shares
  shareList: '[data-testid="share-list"]',
  shareRow: (name: string) => `[data-testid="share-row-${name}"]`,
  createShareButton: '[data-testid="create-share"]',

  // Snapshots
  snapshotList: '[data-testid="snapshot-list"]',
  snapshotRow: (name: string) => `[data-testid="snapshot-row-${name}"]`,
  takeSnapshotButton: '[data-testid="take-snapshot"]',

  // Apps catalog
  catalogGrid: '[data-testid="catalog-grid"]',
  catalogCard: (id: string) => `[data-testid="catalog-${id}"]`,
  installAppButton: '[data-testid="install-app"]',
  appStatus: (name: string) => `[data-testid="app-status-${name}"]`,

  // System
  systemSettings: '[data-testid="system-settings"]',
  updatesPage: '[data-testid="system-updates"]',
  certificatesPage: '[data-testid="system-certificates"]',
  alertsPage: '[data-testid="system-alerts"]',
  auditPage: '[data-testid="system-audit"]',
} as const;
