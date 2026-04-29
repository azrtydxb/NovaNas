import { useState } from "react";
import Active from "./Active";
import Silences from "./Silences";
import Receivers from "./Receivers";

type Tab = "active" | "silences" | "receivers";

export default function Alerts() {
  const [tab, setTab] = useState<Tab>("active");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["active", "silences", "receivers"] as const).map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>
      <div className="win-body" style={{ padding: 0, overflow: "auto", flex: 1 }}>
        {tab === "active" && <Active />}
        {tab === "silences" && <Silences />}
        {tab === "receivers" && <Receivers />}
      </div>
    </div>
  );
}
