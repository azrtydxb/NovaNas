// Control Panel — settings hub.
// Each control-card row is a shortcut to another app. Since registry wiring
// lives in the controller, we dispatch a window CustomEvent
// `nova:open-app` with the target appId. The desktop controller (which owns
// the registry) listens and opens the requested app.

import { Icon, type IconName } from "../../components/Icon";

type Item = { label: string; appId?: string };
type Section = { name: string; icon: IconName; items: Item[] };

const SECTIONS: Section[] = [
  {
    name: "Storage",
    icon: "storage",
    items: [
      { label: "Pools", appId: "storage" },
      { label: "Datasets", appId: "storage" },
      { label: "Snapshots", appId: "storage" },
      { label: "Disks", appId: "storage" },
    ],
  },
  {
    name: "Network",
    icon: "net",
    items: [
      { label: "Interfaces", appId: "network" },
      { label: "RDMA", appId: "network" },
      { label: "Routes", appId: "network" },
      { label: "DNS", appId: "network" },
    ],
  },
  {
    name: "Identity",
    icon: "user",
    items: [
      { label: "Users", appId: "identity" },
      { label: "Groups", appId: "identity" },
      { label: "SSO", appId: "identity" },
      { label: "API tokens", appId: "identity" },
    ],
  },
  {
    name: "System",
    icon: "control",
    items: [
      { label: "Overview", appId: "system" },
      { label: "Updates", appId: "system" },
      { label: "SMTP", appId: "system" },
    ],
  },
  {
    name: "Workloads",
    icon: "workload",
    items: [
      { label: "Apps", appId: "package-center" },
      { label: "Containers", appId: "workloads" },
      { label: "VMs", appId: "vm" },
    ],
  },
  {
    name: "Tools",
    icon: "terminal",
    items: [
      { label: "Terminal", appId: "terminal" },
      { label: "File Station", appId: "files" },
      { label: "Monitor", appId: "monitor" },
    ],
  },
];

function openApp(appId: string | undefined) {
  if (!appId) return;
  window.dispatchEvent(
    new CustomEvent("nova:open-app", { detail: appId })
  );
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
