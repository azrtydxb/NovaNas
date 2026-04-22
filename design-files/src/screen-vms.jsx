/* globals React, Icon, Pill, StatusDot, VMS */
function VmsScreen({ role }) {
  const list = role === "user" ? VMS.filter(v => v.owner === "pascal") : VMS;
  const [sel, setSel] = React.useState(list[0].name);
  const vm = list.find(v => v.name === sel) || list[0];
  const tick = window.useTicker(1200);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{role === "user" ? "My VMs" : "Virtual Machines"}</h1>
          <div className="page-head__sub">{list.filter(v => v.state === "Running").length}/{list.length} running · KubeVirt 1.4</div>
        </div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="download" size={13}/> ISO library</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> New VM</button>
        </div>
      </div>

      <div className="grid" style={{ gridTemplateColumns: "1fr 420px", gap: 14 }}>
        <div className="card">
          <table className="tbl">
            <thead><tr>
              <th>Name</th><th>Owner</th><th>OS</th><th className="num">CPU</th><th className="num">RAM</th><th>GPU</th><th>IP</th><th>State</th>
            </tr></thead>
            <tbody>
              {list.map(v => (
                <tr key={v.name} onClick={() => setSel(v.name)} style={{ cursor: "pointer", background: v.name === sel ? "var(--bg-3)" : undefined }}>
                  <td><span className="fg0" style={{ fontWeight: 500 }}>{v.name}</span></td>
                  <td className="mono">{v.owner}</td>
                  <td>{v.os}</td>
                  <td className="num">{v.cpu}</td>
                  <td className="num">{(v.ramMiB/1024).toFixed(0)} GiB</td>
                  <td>{v.gpu ? <span className="chip">{v.gpu}</span> : <span className="muted">—</span>}</td>
                  <td className="mono muted">{v.ip || "—"}</td>
                  <td>{v.state === "Running" ? <Pill tone="ok" dot>Running</Pill> : <Pill>Stopped</Pill>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Console panel */}
        <aside className="card" style={{ position: "sticky", top: 16, alignSelf: "start" }}>
          <div className="card__head">
            <div className="card__title">{vm.name}</div>
            <div className="card__sub">{vm.os}</div>
            <div className="card__actions">
              {vm.state === "Running"
                ? <button className="btn btn--sm"><Icon name="pause" size={12}/></button>
                : <button className="btn btn--sm btn--primary"><Icon name="play" size={12}/></button>
              }
              <button className="btn btn--sm"><Icon name="restart" size={12}/></button>
              <button className="btn btn--sm"><Icon name="stop" size={12}/></button>
            </div>
          </div>
          <div className="card__body col gap-12">
            <div className="console">
              <div className="console__bar">
                <Icon name="monitor" size={11}/>
                <span>SPICE</span>
                <span className="sep"/>
                <span>{vm.name}</span>
                <span className="sep"/>
                <span>{1920 + (tick % 3)}×1080</span>
                <span style={{ marginLeft: "auto" }}>●&nbsp;rec</span>
              </div>
              <MockDesktop os={vm.os} on={vm.state === "Running"}/>
            </div>

            <div className="grid grid-2 gap-8">
              <Stat label="vCPU" value={vm.cpu} unit=""/>
              <Stat label="RAM" value={(vm.ramMiB/1024).toFixed(0)} unit="GiB"/>
              <Stat label="Disks" value={vm.disks} unit=""/>
              <Stat label="GPU" value={vm.gpu || "none"}/>
            </div>

            <div className="col gap-4">
              <div className="muted" style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.06em" }}>Disks</div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}>
                <span className="mono">system · dataset {vm.owner}/win-system</span>
                <span className="mono muted">80 GiB virtio</span>
              </div>
              {vm.disks > 1 && (
                <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}>
                  <span className="mono">data · blockVolume {vm.owner}/win-data</span>
                  <span className="mono muted">500 GiB virtio</span>
                </div>
              )}
            </div>

            <div className="row gap-8">
              <button className="btn stretch"><Icon name="console" size={12}/> Open in new tab</button>
              <button className="btn"><Icon name="snap" size={12}/></button>
            </div>
          </div>
        </aside>
      </div>
    </>
  );
}

function Stat({ label, value, unit }) {
  return (
    <div style={{ background: "var(--bg-0)", padding: "8px 10px", border: "1px solid var(--line)", borderRadius: "var(--r-sm)" }}>
      <div style={{ fontSize: 10, color: "var(--fg-3)", textTransform: "uppercase", letterSpacing: "0.06em" }}>{label}</div>
      <div className="mono fg0" style={{ fontSize: 15, marginTop: 2 }}>{value}{unit && <span style={{ color: "var(--fg-3)", fontSize: 12, marginLeft: 4 }}>{unit}</span>}</div>
    </div>
  );
}

function MockDesktop({ os, on }) {
  if (!on) {
    return <div className="os" style={{ background: "#000", position: "absolute", inset: 0 }}>
      <span>⏻ Powered off</span>
    </div>;
  }
  // Minimal ambient desktop mock — abstracted, not a real OS
  const isWin = os?.toLowerCase().includes("win");
  const accent = isWin ? "oklch(0.55 0.14 220)" : "oklch(0.55 0.14 30)";
  return (
    <div style={{ position: "absolute", inset: 0, background: `radial-gradient(80% 120% at 30% 30%, ${accent}, oklch(0.12 0.02 250))`, overflow: "hidden" }}>
      {/* window rects */}
      <div style={{ position: "absolute", left: "8%", top: "14%", width: "48%", height: "56%", background: "oklch(0.22 0.008 250 / 0.9)", border: "1px solid oklch(1 0 0 / 0.12)", borderRadius: 8, backdropFilter: "blur(8px)" }}>
        <div style={{ height: 18, background: "oklch(1 0 0 / 0.08)", display: "flex", gap: 4, alignItems: "center", padding: "0 8px", fontSize: 9, color: "oklch(0.9 0 0)", fontFamily: "var(--font-mono)" }}>
          <span style={{ width: 7, height: 7, borderRadius: "50%", background: "oklch(0.72 0.17 25)" }}/>
          <span style={{ width: 7, height: 7, borderRadius: "50%", background: "oklch(0.82 0.14 80)" }}/>
          <span style={{ width: 7, height: 7, borderRadius: "50%", background: "oklch(0.72 0.14 150)" }}/>
          <span style={{ marginLeft: 8 }}>terminal</span>
        </div>
        <div style={{ padding: 8, fontFamily: "var(--font-mono)", fontSize: 9, color: "oklch(0.85 0 0)" }}>
          <div style={{ color: "oklch(0.78 0.14 160)" }}>$ uname -a</div>
          <div>Linux dev 6.8.0 #1 SMP x86_64 GNU/Linux</div>
          <div style={{ color: "oklch(0.78 0.14 160)" }}>$ systemctl status</div>
          <div>● running (22/22 units)</div>
        </div>
      </div>
      <div style={{ position: "absolute", right: "6%", top: "26%", width: "34%", height: "40%", background: "oklch(0.22 0.008 250 / 0.9)", border: "1px solid oklch(1 0 0 / 0.12)", borderRadius: 8 }}>
        <div style={{ height: 18, background: "oklch(1 0 0 / 0.08)" }}/>
        <div style={{ padding: 8, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
          {[...Array(6)].map((_, i) => (
            <div key={i} style={{ aspectRatio: "4/3", background: `oklch(${0.3 + i*0.05} 0.05 ${200 + i*20})`, borderRadius: 3 }}/>
          ))}
        </div>
      </div>
      {/* taskbar */}
      <div style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: 20, background: "oklch(0 0 0 / 0.35)", backdropFilter: "blur(10px)", display: "flex", alignItems: "center", gap: 6, padding: "0 8px", fontFamily: "var(--font-mono)", fontSize: 9, color: "oklch(0.9 0 0)" }}>
        <span>●</span>
        <span style={{ marginLeft: "auto" }}>14:02</span>
      </div>
    </div>
  );
}

window.VmsScreen = VmsScreen;
