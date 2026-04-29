import { useState } from "react";
import { Releases } from "./Releases";
import { Catalog } from "./Catalog";
import { Events } from "./Events";

type Tab = "releases" | "catalog" | "events";
const TABS: Tab[] = ["releases", "catalog", "events"];

export default function Workloads() {
  const [tab, setTab] = useState<Tab>("releases");
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
        {tab === "releases" && <Releases />}
        {tab === "catalog" && <Catalog />}
        {tab === "events" && <Events />}
      </div>
    </div>
  );
}
