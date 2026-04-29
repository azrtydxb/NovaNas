// Control Panel — settings hub. Layout matches design exactly: 6 cards
// (Network, Identity, Security, Hardware, Notifications, Backup), each with
// an icon, name, and clickable items. Items dispatch `nova:open-app` to the
// desktop controller (which owns the registry) so this remains data-only.

import { Icon, type IconName } from "../../components/Icon";

type Item = { label: string; appId?: string };
type Section = { name: string; icon: IconName; items: Item[] };

const SECTIONS: Section[] = [
  {
    name: "Network",
    icon: "net",
    items: [
      { label: "Interfaces", appId: "network" },
      { label: "Routes", appId: "network" },
      { label: "DNS", appId: "network" },
      { label: "Firewall", appId: "network" },
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
    name: "Security",
    icon: "shield",
    items: [
      { label: "Encryption", appId: "storage" },
      { label: "Certificates", appId: "system" },
      { label: "2FA", appId: "identity" },
      { label: "Audit", appId: "audit" },
    ],
  },
  {
    name: "Hardware",
    icon: "cpu",
    items: [
      { label: "CPU & Power", appId: "system" },
      { label: "Fans", appId: "system" },
      { label: "UPS", appId: "system" },
      { label: "Sensors", appId: "system" },
    ],
  },
  {
    name: "Notifications",
    icon: "bell",
    items: [
      { label: "Channels", appId: "notifications" },
      { label: "Rules", appId: "alerts" },
      { label: "Webhooks", appId: "system" },
    ],
  },
  {
    name: "Backup",
    icon: "download",
    items: [
      { label: "Replication", appId: "replication" },
      { label: "Cloud sync", appId: "replication" },
      { label: "Schedule", appId: "jobs" },
    ],
  },
];

function openApp(appId?: string) {
  if (!appId) return;
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
