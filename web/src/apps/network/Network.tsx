import { useState } from "react";
import { Interfaces } from "./Interfaces";
import { RDMA } from "./RDMA";

type Tab = "interfaces" | "rdma";

export function Network() {
  const [tab, setTab] = useState<Tab>("interfaces");

  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["interfaces", "rdma"] as const).map((t) => (
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
        {tab === "interfaces" && <Interfaces />}
        {tab === "rdma" && <RDMA />}
      </div>
    </div>
  );
}

export default Network;
