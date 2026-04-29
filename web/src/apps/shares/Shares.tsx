import { useState } from "react";
import { Unified } from "./Unified";
import { SMB } from "./SMB";
import { NFS } from "./NFS";
import { ISCSI } from "./ISCSI";
import { NVMEOF } from "./NVMEOF";

type Tab = "unified" | "smb" | "nfs" | "iscsi" | "nvmeof";

export function Shares() {
  const [tab, setTab] = useState<Tab>("unified");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["unified", "smb", "nfs", "iscsi", "nvmeof"] as const).map((t) => (
          <button
            key={t}
            className={tab === t ? "is-on" : ""}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      <div className="win-body" style={{ padding: 0, overflow: "auto" }}>
        {tab === "unified" && <Unified />}
        {tab === "smb" && <SMB />}
        {tab === "nfs" && <NFS />}
        {tab === "iscsi" && <ISCSI />}
        {tab === "nvmeof" && <NVMEOF />}
      </div>
    </div>
  );
}

export default Shares;
