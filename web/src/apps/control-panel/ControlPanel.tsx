// Control Panel — settings hub.
// Each control-card row is a shortcut to another app. Since registry wiring
// lives in the controller, we dispatch a window CustomEvent
// `nova:open-app` with the target appId. The desktop controller (which owns
// the registry) listens and opens the requested app.

import { Icon, type IconName } from "../../components/Icon";

type Item = { label: string; appId: string };
type Section = { name: string; icon: IconName; items: Item[] };

const SECTIONS: Section[] = [
  {
    name: "Storage",
    icon: "storage",
    items: [
      { label: "Pools", appId: "storage" },
      { label: "Datasets", appId: "storage" },
      { label: "Snapshots", appId: "storage" },
      { label: "Encryption", appId: "storage" },
      { label: "Replication", appId: "replication" },
      { label: "Shares", appId: "shares" },
    ],
  },
  {
    name: "Network",
    icon: "net",
    items: [
      { label: "Interfaces", appId: "network" },
      { label: "RDMA", appId: "network" },
    ],
  },
  {
    name: "Identity",
    icon: "user",
    items: [
      { label: "Users", appId: "identity" },
      { label: "Sessions", appId: "identity" },
      { label: "Kerberos", appId: "identity" },
    ],
  },
  {
    name: "System",
    icon: "settings",
    items: [
      { label: "Overview", appId: "system" },
      { label: "Update", appId: "system" },
      { label: "SMTP", appId: "system" },
    ],
  },
  {
    name: "Apps",
    icon: "package",
    items: [
      { label: "Package Center", appId: "package-center" },
      { label: "Workloads", appId: "workloads" },
      { label: "Virtualization", appId: "vms" },
    ],
  },
  {
    name: "Tools",
    icon: "control",
    items: [
      { label: "Alerts", appId: "alerts" },
      { label: "Logs", appId: "logs" },
      { label: "Audit", appId: "audit" },
      { label: "Jobs", appId: "jobs" },
      { label: "Notifications", appId: "notifications" },
      { label: "File Station", appId: "files" },
      { label: "Terminal", appId: "terminal" },
    ],
  },
];

function openApp(appId: string) {
  window.dispatchEvent(new CustomEvent("nova:open-app", { detail: appId }));
}

export function ControlPanel() {
  return (
    <div className="app-control">
      {SECTIONS.map((s) => (
        <div key={s.name} className="control-card">
          <div className="control-card__head">
            <div className="control-card__icon">
              <Icon name={s.icon} size={16} />
            </div>
            <div className="control-card__name">{s.name}</div>
          </div>
          <ul className="control-card__items">
            {s.items.map((it) => (
              <li
                key={it.label}
                onClick={() => openApp(it.appId)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") openApp(it.appId);
                }}
              >
                {it.label}
                <Icon name="chev" size={12} style={{ opacity: 0.4 }} />
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

export default ControlPanel;
