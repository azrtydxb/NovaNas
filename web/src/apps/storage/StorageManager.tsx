import { useState } from "react";
import { PoolsTab } from "./PoolsTab";
import { VdevsTab } from "./VdevsTab";
import { DatasetsTab } from "./DatasetsTab";
import { SnapshotsTab } from "./SnapshotsTab";
import { DisksTab } from "./DisksTab";
import { EncryptionTab } from "./EncryptionTab";

type Tab = "pools" | "vdev" | "disks" | "datasets" | "snapshots" | "encryption";

export function StorageManager() {
  const [tab, setTab] = useState<Tab>("pools");
  const [poolSel, setPoolSel] = useState<string | null>(null);

  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["pools", "vdev", "disks", "datasets", "snapshots", "encryption"] as const).map(
          (t) => (
            <button
              key={t}
              className={tab === t ? "is-on" : ""}
              onClick={() => setTab(t)}
            >
              {t}
            </button>
          )
        )}
      </div>
      <div className="win-body" style={{ padding: 0, overflow: "auto" }}>
        {tab === "pools" && (
          <PoolsTab
            onPick={(n) => {
              setPoolSel(n);
              setTab("vdev");
            }}
          />
        )}
        {tab === "vdev" && <VdevsTab pool={poolSel} setPool={setPoolSel} />}
        {tab === "disks" && <DisksTab />}
        {tab === "datasets" && <DatasetsTab />}
        {tab === "snapshots" && <SnapshotsTab />}
        {tab === "encryption" && <EncryptionTab />}
      </div>
    </div>
  );
}

export default StorageManager;
