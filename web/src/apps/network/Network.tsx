import { useState } from "react";
import { Interfaces } from "./Interfaces";
import { Routes } from "./Routes";
import { RDMA } from "./RDMA";

type Tab = "interfaces" | "rdma" | "routes";

export function Network() {
  const [tab, setTab] = useState<Tab>("interfaces");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["interfaces", "rdma", "routes"] as const).map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>
      <div className="win-body" style={{ padding: 0, overflow: "auto" }}>
        {tab === "interfaces" && <Interfaces />}
        {tab === "rdma" && <RDMA />}
        {tab === "routes" && <Routes />}
      </div>
    </div>
  );
}

export default Network;
