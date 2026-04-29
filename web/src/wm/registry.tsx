import { lazy, Suspense, type ComponentType } from "react";
import { Icon, type IconName } from "../components/Icon";
import type { AppDef, AppId } from "./types";

// PackageCenter is local (built first, in this conversation).
import { PackageCenter } from "../apps/package-center/PackageCenter";

// The remaining apps are built by parallel agents. Import them lazily
// so a missing module turns into a graceful fallback instead of a
// build failure during integration. Once an agent ships its module,
// drop the catch fallback and the app renders for real.
function lazyApp(loader: () => Promise<{ default: ComponentType } | Record<string, ComponentType>>, named: string) {
  const Lazy = lazy(async () => {
    try {
      const mod = await loader();
      const C = ("default" in mod ? mod.default : (mod as Record<string, ComponentType>)[named]) as ComponentType | undefined;
      if (!C) throw new Error(`No export "${named}" in module`);
      return { default: C };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      const Stub: ComponentType = () => <ComingSoon name={named} reason={msg} />;
      return { default: Stub };
    }
  });
  return function Wrapped() {
    return (
      <Suspense fallback={<div className="app-loading">Loading {named}…</div>}>
        <Lazy />
      </Suspense>
    );
  };
}

function ComingSoon({ name, reason }: { name: string; reason?: string }) {
  return (
    <div className="app-coming-soon">
      <div className="app-coming-soon__title">{name}</div>
      <div className="app-coming-soon__sub muted">
        Implementation lands in the next deploy.
      </div>
      {reason && <pre className="app-coming-soon__err mono muted">{reason}</pre>}
    </div>
  );
}

const make = (id: AppId, title: string, icon: IconName, w: number, h: number, Component: ComponentType): AppDef => ({
  id, title, icon, defaultSize: { w, h }, Component,
});

export const APPS: Record<AppId, AppDef> = {
  "package-center": make("package-center", "Package Center", "package", 1080, 660, PackageCenter),
  storage:          make("storage",         "Storage Manager", "storage",     1080, 660, lazyApp(() => import("../apps/storage/StorageManager"), "StorageManager")),
  replication:      make("replication",     "Replication",     "replication", 1000, 620, lazyApp(() => import("../apps/replication/Replication"), "Replication")),
  shares:           make("shares",          "Shares",          "share",       1040, 620, lazyApp(() => import("../apps/shares/Shares"), "Shares")),
  identity:         make("identity",        "Identity",        "user",        1000, 620, lazyApp(() => import("../apps/identity/Identity"), "Identity")),
  workloads:        make("workloads",       "Workloads",       "workload",    1000, 620, lazyApp(() => import("../apps/workloads/Workloads"), "Workloads")),
  vms:              make("vms",             "Virtualization",  "vm",          1040, 640, lazyApp(() => import("../apps/vms/Virtualization"), "Virtualization")),
  alerts:           make("alerts",          "Alerts",          "alert",        960, 600, lazyApp(() => import("../apps/alerts/Alerts"), "Alerts")),
  logs:             make("logs",            "Logs",            "log",         1040, 620, lazyApp(() => import("../apps/logs/Logs"), "Logs")),
  audit:            make("audit",           "Audit",           "audit",        960, 600, lazyApp(() => import("../apps/audit/Audit"), "Audit")),
  jobs:             make("jobs",            "Jobs",            "job",          960, 600, lazyApp(() => import("../apps/jobs/Jobs"), "Jobs")),
  notifications:    make("notifications",   "Notifications",   "bell",         760, 600, lazyApp(() => import("../apps/notifications/Notifications"), "Notifications")),
  network:          make("network",         "Network",         "net",          960, 600, lazyApp(() => import("../apps/network/Network"), "Network")),
  system:           make("system",          "System",          "settings",     960, 600, lazyApp(() => import("../apps/system/System"), "System")),
  files:            make("files",           "File Station",    "files",       1040, 620, lazyApp(() => import("../apps/files/FileStation"), "FileStation")),
  terminal:         make("terminal",        "Terminal",        "terminal",     820, 500, lazyApp(() => import("../apps/terminal/Terminal"), "Terminal")),
  "control-panel":  make("control-panel",   "Control Panel",   "control",      820, 560, lazyApp(() => import("../apps/control-panel/ControlPanel"), "ControlPanel")),
};

// Order matters for the launcher grid + dock.
export const APP_LIST: AppDef[] = [
  APPS["package-center"],
  APPS.storage, APPS.replication, APPS.shares,
  APPS.identity, APPS.workloads, APPS.vms,
  APPS.alerts, APPS.logs, APPS.audit, APPS.jobs, APPS.notifications,
  APPS.network, APPS.system, APPS.files, APPS.terminal, APPS["control-panel"],
];

// Re-export Icon so other files don't need a long relative path; not
// strictly necessary but tidy.
export { Icon };
