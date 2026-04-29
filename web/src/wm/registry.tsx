import { PackageCenter } from "../apps/package-center/PackageCenter";
import type { AppDef, AppId } from "./types";

const Stub = ({ name }: { name: string }) => (
  <div style={{ padding: 24, color: "var(--fg-2)" }}>
    <h3 style={{ marginTop: 0 }}>{name}</h3>
    <p style={{ fontSize: 12 }}>Coming in a later phase.</p>
  </div>
);

const stub = (name: string) => () => <Stub name={name} />;

export const APPS: Record<AppId, AppDef> = {
  "package-center": {
    id: "package-center",
    title: "Package Center",
    icon: "package",
    defaultSize: { w: 1080, h: 660 },
    Component: PackageCenter,
  },
  storage: {
    id: "storage",
    title: "Storage Manager",
    icon: "storage",
    defaultSize: { w: 1080, h: 660 },
    Component: stub("Storage Manager"),
  },
  replication: { id: "replication", title: "Replication", icon: "replication", defaultSize: { w: 960, h: 600 }, Component: stub("Replication") },
  shares: { id: "shares", title: "Shares", icon: "share", defaultSize: { w: 1000, h: 600 }, Component: stub("Shares") },
  identity: { id: "identity", title: "Identity", icon: "user", defaultSize: { w: 960, h: 600 }, Component: stub("Identity") },
  workloads: { id: "workloads", title: "Workloads", icon: "workload", defaultSize: { w: 960, h: 600 }, Component: stub("Workloads") },
  vms: { id: "vms", title: "Virtualization", icon: "vm", defaultSize: { w: 960, h: 600 }, Component: stub("Virtualization") },
  alerts: { id: "alerts", title: "Alerts", icon: "alert", defaultSize: { w: 900, h: 580 }, Component: stub("Alerts") },
  logs: { id: "logs", title: "Logs", icon: "log", defaultSize: { w: 1000, h: 580 }, Component: stub("Logs") },
  audit: { id: "audit", title: "Audit", icon: "audit", defaultSize: { w: 900, h: 560 }, Component: stub("Audit") },
  jobs: { id: "jobs", title: "Jobs", icon: "job", defaultSize: { w: 900, h: 560 }, Component: stub("Jobs") },
  notifications: { id: "notifications", title: "Notifications", icon: "bell", defaultSize: { w: 720, h: 560 }, Component: stub("Notifications") },
  network: { id: "network", title: "Network", icon: "network", defaultSize: { w: 900, h: 560 }, Component: stub("Network") },
  system: { id: "system", title: "System", icon: "system", defaultSize: { w: 900, h: 560 }, Component: stub("System") },
  files: { id: "files", title: "File Station", icon: "file", defaultSize: { w: 1000, h: 600 }, Component: stub("File Station") },
  terminal: { id: "terminal", title: "Terminal", icon: "terminal", defaultSize: { w: 800, h: 480 }, Component: stub("Terminal") },
  "control-panel": { id: "control-panel", title: "Control Panel", icon: "settings", defaultSize: { w: 800, h: 540 }, Component: stub("Control Panel") },
};

export const APP_LIST: AppDef[] = Object.values(APPS);
