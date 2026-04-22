/* globals React, ReactDOM, Topbar, Rail, DashboardScreen, PoolsScreen, DisksScreen, DatasetsScreen, SharesScreen, AppsScreen, VmsScreen, NetworkScreen, IdentityScreen, SystemScreen, TweaksPanel */
const { useState: useS, useEffect: useE, useMemo: useM } = React;

function App() {
  const initialTweaks = window.__NOVANAS_TWEAKS__ || { theme: "dark", density: "dense", accentHue: 220, role: "admin", seed: "healthy" };
  const [tweaks, setTweaks] = useS(initialTweaks);
  const [route, setRoute] = useS(() => localStorage.getItem("nn.route") || "dashboard");
  const [tweaksOpen, setTweaksOpen] = useS(false);
  const [editMode, setEditMode] = useS(false);

  useE(() => { localStorage.setItem("nn.route", route); }, [route]);

  // apply theme / density / accent to <body>
  useE(() => {
    document.body.classList.toggle("theme-dark", tweaks.theme === "dark");
    document.body.classList.toggle("theme-light", tweaks.theme === "light");
    document.body.classList.toggle("density-dense", tweaks.density === "dense");
    document.body.classList.toggle("density-spacious", tweaks.density === "spacious");
    document.documentElement.style.setProperty("--accent-h", String(tweaks.accentHue));
  }, [tweaks]);

  // edit-mode protocol
  useE(() => {
    const handler = (e) => {
      const d = e.data;
      if (!d || typeof d !== "object") return;
      if (d.type === "__activate_edit_mode")   { setEditMode(true); setTweaksOpen(true); }
      if (d.type === "__deactivate_edit_mode") { setEditMode(false); setTweaksOpen(false); }
    };
    window.addEventListener("message", handler);
    try { window.parent.postMessage({ type: "__edit_mode_available" }, "*"); } catch (e) {}
    return () => window.removeEventListener("message", handler);
  }, []);

  const role = tweaks.role;
  const effectiveRoute = useM(() => {
    const userAllowed = ["dashboard", "datasets", "shares", "snapshots", "apps", "vms"];
    if (role === "user" && !userAllowed.includes(route)) return "dashboard";
    return route;
  }, [role, route]);

  const Screen = () => {
    switch (effectiveRoute) {
      case "dashboard": return <DashboardScreen seed={tweaks.seed} role={role}/>;
      case "pools":     return <PoolsScreen/>;
      case "disks":     return <DisksScreen seed={tweaks.seed}/>;
      case "datasets":  return <DatasetsScreen role={role}/>;
      case "snapshots": return <SnapshotsStub/>;
      case "shares":    return <SharesScreen/>;
      case "iscsi":     return <SimpleStub title="iSCSI / NVMe-oF" sub="Block targets for VM disks and bare-metal iSCSI initiators."/>;
      case "s3":        return <SimpleStub title="S3" sub="Native chunk-engine object storage (ObjectStore, Buckets, BucketUsers)."/>;
      case "protect":   return <SimpleStub title="Data Protection" sub="Snapshot schedules, replication, cloud backup."/>;
      case "apps":      return <AppsScreen role={role}/>;
      case "vms":       return <VmsScreen role={role}/>;
      case "network":   return <NetworkScreen/>;
      case "identity":  return <IdentityScreen/>;
      case "system":    return <SystemScreen/>;
      default:          return <DashboardScreen seed={tweaks.seed} role={role}/>;
    }
  };

  return (
    <div className="app">
      <Topbar route={effectiveRoute} setRoute={setRoute} role={role}
        setRole={(r) => setTweaks({ ...tweaks, role: r })}
        onTweaks={() => setTweaksOpen(v => !v)}
      />
      <Rail route={effectiveRoute} setRoute={setRoute} role={role}/>
      <main className="main">
        <Screen/>
      </main>
      <TweaksPanel tweaks={tweaks} setTweaks={setTweaks} open={tweaksOpen} setOpen={setTweaksOpen}/>
    </div>
  );
}

function SnapshotsStub() {
  const rows = [
    ["family-media@auto-14:58", "family-media", "1.4 GB", "auto", "14:58"],
    ["pascal/docs@pre-update-001", "pascal/docs", "24 MB", "pre-update", "14:02"],
    ["vm-disks@win11-pre-update", "vm-disks", "2.1 GB", "pre-update", "12:04"],
    ["family-media@auto-13:58", "family-media", "1.2 GB", "auto", "13:58"],
    ["backups@weekly-2026-17", "backups", "412 MB", "weekly", "Sun 03:00"],
  ];
  return (
    <>
      <div className="page-head">
        <div><h1>Snapshots</h1><div className="page-head__sub">Immutable, chunk-deduplicated. Retention via SnapshotSchedule.</div></div>
        <div className="page-head__actions"><button className="btn btn--primary"><window.Icon name="snap" size={13}/> New snapshot</button></div>
      </div>
      <div className="card">
        <table className="tbl">
          <thead><tr><th>Name</th><th>Dataset</th><th className="num">Size</th><th>Source</th><th>Taken</th><th></th></tr></thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i}>
                <td className="mono fg0">{r[0]}</td>
                <td>{r[1]}</td>
                <td className="num">{r[2]}</td>
                <td><span className="chip">{r[3]}</span></td>
                <td className="mono muted">{r[4]}</td>
                <td><button className="btn btn--ghost btn--sm"><window.Icon name="more" size={12}/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function SimpleStub({ title, sub }) {
  return (
    <>
      <div className="page-head"><div><h1>{title}</h1><div className="page-head__sub">{sub}</div></div></div>
      <div className="card" style={{ padding: 40, textAlign: "center", color: "var(--fg-3)" }}>
        <div style={{ fontSize: 13 }}>This surface is part of the full UI; not wireframed in this pass.</div>
        <div className="hint mt-8">Jump to <a className="lnk" onClick={() => document.location.reload()}>Dashboard</a>.</div>
      </div>
    </>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App/>);
