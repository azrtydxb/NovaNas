import { useState } from "react";
import { Overview } from "./Overview";
import { Update } from "./Update";
import { SMTP } from "./SMTP";

type Tab = "overview" | "updates" | "smtp";

export function System() {
  const [tab, setTab] = useState<Tab>("overview");

  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["overview", "updates", "smtp"] as const).map((t) => (
          <button
            key={t}
            className={tab === t ? "is-on" : ""}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        {tab === "overview" && <Overview />}
        {tab === "updates" && <Update />}
        {tab === "smtp" && <SMTP />}
      </div>
    </div>
  );
}

export default System;
