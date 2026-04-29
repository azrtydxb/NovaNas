import { useState } from "react";
import { Jobs } from "./Jobs";
import { Targets } from "./Targets";
import { Schedules } from "./Schedules";
import { Scrub } from "./Scrub";

type Tab = "jobs" | "targets" | "schedules" | "scrub";

export function Replication() {
  const [tab, setTab] = useState<Tab>("jobs");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["jobs", "targets", "schedules", "scrub"] as const).map((t) => (
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
        {tab === "jobs" && <Jobs />}
        {tab === "targets" && <Targets />}
        {tab === "schedules" && <Schedules />}
        {tab === "scrub" && <Scrub />}
      </div>
    </div>
  );
}

export default Replication;
