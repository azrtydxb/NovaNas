import { useState } from "react";
import { Discover } from "./Discover";
import { Installed } from "./Installed";
import { Marketplaces } from "./Marketplaces";

type Tab = "discover" | "installed" | "marketplaces";

export function PackageCenter() {
  const [tab, setTab] = useState<Tab>("discover");
  return (
    <div className="app-pkg">
      <div className="win-tabs">
        {(["discover", "installed", "marketplaces"] as const).map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>
      <div className="app-pkg__body">
        {tab === "discover" && <Discover />}
        {tab === "installed" && <Installed />}
        {tab === "marketplaces" && <Marketplaces />}
      </div>
    </div>
  );
}
