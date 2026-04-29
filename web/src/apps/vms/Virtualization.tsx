import { useState } from "react";
import { VMs } from "./VMs";
import { Templates } from "./Templates";
import { VMSnapshots } from "./VMSnapshots";

type Tab = "vms" | "templates" | "snapshots";
const TABS: Tab[] = ["vms", "templates", "snapshots"];

export default function Virtualization() {
  const [tab, setTab] = useState<Tab>("vms");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {TABS.map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>
      <div className="win-body" style={{ padding: 0, overflow: "auto" }}>
        {tab === "vms" && <VMs />}
        {tab === "templates" && <Templates />}
        {tab === "snapshots" && <VMSnapshots />}
      </div>
    </div>
  );
}
