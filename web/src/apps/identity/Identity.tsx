import { useState } from "react";
import { Users } from "./Users";
import { Sessions } from "./Sessions";
import { LoginHistory } from "./LoginHistory";
import { Kerberos } from "./Kerberos";

type Tab = "users" | "sessions" | "login-history" | "kerberos";
const TABS: Tab[] = ["users", "sessions", "login-history", "kerberos"];

export default function Identity() {
  const [tab, setTab] = useState<Tab>("users");
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
        {tab === "users" && <Users />}
        {tab === "sessions" && <Sessions />}
        {tab === "login-history" && <LoginHistory />}
        {tab === "kerberos" && <Kerberos />}
      </div>
    </div>
  );
}
