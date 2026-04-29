/* globals React, Icon, Spark, useSeries, Ring, POOLS, DISKS, DATASETS, APPS, VMS, FILES, ACTIVITY, fmtBytes, fmtPct */

// ═══════════════ STORAGE MANAGER ═══════════════
function StorageManager() {
  const [tab, setTab] = React.useState("pools");
  const [sel, setSel] = React.useState(null);
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["pools","disks","datasets","snapshots"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={() => setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body">
        {tab === "pools" && <PoolsTab/>}
        {tab === "disks" && <DisksTab sel={sel} setSel={setSel}/>}
        {tab === "datasets" && <DatasetsTab/>}
        {tab === "snapshots" && <SnapshotsTab/>}
      </div>
    </div>
  );
}

function PoolsTab() {
  return (
    <div className="cards-grid">
      {POOLS.map(p => {
        const pct = p.used / p.total;
        return (
          <div key={p.name} className="pool-card">
            <div className="pool-card__head">
              <div className="pool-card__name">
                <Icon name="storage" size={14}/>
                <span>{p.name}</span>
                <span className={`tier tier--${p.tier}`}>{p.tier}</span>
              </div>
              <span className="pill pill--ok"><span className="dot"/>Healthy</span>
            </div>
            <div className="pool-card__meta">
              <span>{p.disks} disks · {p.devices}</span>
              <span>{p.protection}</span>
            </div>
            <div className="bar"><div style={{ width: `${pct*100}%` }}/></div>
            <div className="pool-card__nums">
              <span className="mono">{fmtBytes(p.used)} / {fmtBytes(p.total)}</span>
              <span className="muted mono">{fmtPct(pct)}</span>
            </div>
            <div className="pool-card__io">
              <div><span className="muted">R</span> <span className="mono">{p.throughput.r} MB/s</span></div>
              <div><span className="muted">W</span> <span className="mono">{p.throughput.w} MB/s</span></div>
              <div><span className="muted">IOPS</span> <span className="mono">{(p.iops.r/1000).toFixed(1)}k</span></div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function DisksTab({ sel, setSel }) {
  const selected = DISKS.find(d => d.slot === sel);
  return (
    <div className="disks-split">
      <div>
        <div className="encl-title">ENCLOSURE 0 · 24-bay</div>
        <div className="encl-grid">
          {DISKS.map(d => (
            <div key={d.slot}
              className={`encl-slot ${sel===d.slot?"is-on":""} ${d.state==="EMPTY"?"is-empty":""}`}
              data-state={d.state}
              onClick={() => setSel(d.slot)}>
              <div className="encl-slot__top">
                <span className="mono num">{String(d.slot).padStart(2,"0")}</span>
                <span className="led"/>
              </div>
              <div className="encl-slot__bot">
                {d.state === "EMPTY" ? <span className="muted mono" style={{fontSize:9}}>empty</span> : (
                  <>
                    <span className="mono" style={{fontSize:10, color:"var(--fg-2)"}}>{d.model.split(" ").slice(-1)[0]}</span>
                    <span className="mono" style={{fontSize:10, color:"var(--fg-0)"}}>{fmtBytes(d.cap)}</span>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="disk-detail">
        {!selected ? (
          <div className="empty-hint">Select a slot to inspect</div>
        ) : selected.state === "EMPTY" ? (
          <>
            <div className="disk-detail__head">
              <div>
                <div className="muted mono" style={{fontSize:11}}>SLOT {String(selected.slot).padStart(2,"0")}</div>
                <div className="disk-detail__title">Empty</div>
              </div>
            </div>
            <div className="muted">No disk present in this slot.</div>
            <button className="btn btn--primary">Initialize when inserted</button>
          </>
        ) : (
          <>
            <div className="disk-detail__head">
              <div>
                <div className="muted mono" style={{fontSize:11}}>SLOT {String(selected.slot).padStart(2,"0")} · {selected.class}</div>
                <div className="disk-detail__title">{selected.model}</div>
              </div>
              <span className={`pill pill--${selected.state==="DEGRADED"?"warn":selected.state==="ACTIVE"?"ok":"info"}`}>
                <span className="dot"/>{selected.state}
              </span>
            </div>
            <dl className="kv">
              <dt>Capacity</dt><dd>{fmtBytes(selected.cap)}</dd>
              <dt>Pool</dt><dd>{selected.pool}</dd>
              <dt>Temperature</dt><dd>{selected.temp}°C</dd>
              <dt>Power-on hours</dt><dd>{selected.hours.toLocaleString()}</dd>
              {selected.reason && <><dt>SMART</dt><dd style={{color:"var(--warn)"}}>{selected.reason}</dd></>}
            </dl>
            <div className="row gap-8">
              <button className="btn btn--sm">Run SMART</button>
              <button className="btn btn--sm">Locate</button>
              <button className="btn btn--sm btn--danger">Eject</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function DatasetsTab() {
  return (
    <table className="tbl">
      <thead><tr>
        <th>Dataset</th><th>Pool</th><th>Protocol</th><th className="num">Used</th><th>Quota</th><th className="num">Snaps</th><th>Enc</th>
      </tr></thead>
      <tbody>
        {DATASETS.map(d => {
          const pct = d.used/d.quota;
          return (
            <tr key={d.name}>
              <td><Icon name="files" size={12} style={{verticalAlign:"-2px",marginRight:6,opacity:0.6}}/>{d.name}</td>
              <td className="muted mono">{d.pool}</td>
              <td className="muted">{d.proto}</td>
              <td className="num mono">{fmtBytes(d.used)}</td>
              <td>
                <div className="cap">
                  <div className="cap__bar"><div style={{width:`${pct*100}%`}}/></div>
                  <span className="mono" style={{fontSize:11,color:"var(--fg-3)"}}>{fmtBytes(d.quota)}</span>
                </div>
              </td>
              <td className="num mono">{d.snap}</td>
              <td>{d.enc ? <Icon name="shield" size={12}/> : <span className="muted">—</span>}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function SnapshotsTab() {
  const snaps = [
    { name: "family-media@auto-14:58", pool: "bulk", size: 1.4e9, t: "2 min ago" },
    { name: "pascal/docs@daily-04:00", pool: "fast", size: 240e6, t: "10 h ago" },
    { name: "vm-disks@pre-update", pool: "fast", size: 4.1e9, t: "yesterday" },
    { name: "backups@weekly-W17", pool: "archive", size: 18e9, t: "3 d ago" },
    { name: "pascal/photos@trip", pool: "bulk", size: 3.2e9, t: "1 w ago" },
  ];
  return (
    <table className="tbl">
      <thead><tr><th>Snapshot</th><th>Pool</th><th className="num">Size</th><th>Created</th><th></th></tr></thead>
      <tbody>{snaps.map(s => (
        <tr key={s.name}>
          <td className="mono" style={{fontSize:12}}>{s.name}</td>
          <td className="muted mono">{s.pool}</td>
          <td className="num mono">{fmtBytes(s.size)}</td>
          <td className="muted">{s.t}</td>
          <td className="num"><button className="btn btn--sm">Restore</button></td>
        </tr>
      ))}</tbody>
    </table>
  );
}

// ═══════════════ APP CENTER ═══════════════
// `a.cat` is a backend displayCategory id (lowercase). For the prototype's
// chrome we capitalize for display; production reads the {id, displayName}
// pairs from GET /api/v1/plugins/categories instead.
const catLabel = id => id.charAt(0).toUpperCase() + id.slice(1);
function AppCenter() {
  const [cat, setCat] = React.useState("All");
  const ids = Array.from(new Set(APPS.map(a => a.cat)));
  return (
    <div className="app-appcenter">
      <div className="appcenter-rail">
        {["All", "Installed"].map(c => (
          <button key={c} className={cat===c?"is-on":""} onClick={() => setCat(c)}>{c}</button>
        ))}
        {ids.map(id => (
          <button key={id} className={cat===id?"is-on":""} onClick={() => setCat(id)}>{catLabel(id)}</button>
        ))}
      </div>
      <div className="appcenter-body">
        <div className="appcenter-search">
          <Icon name="search" size={13}/>
          <input placeholder="Search apps..."/>
        </div>
        <div className="appcards">
          {(cat === "All" ? APPS : cat === "Installed" ? APPS.filter(a => a.installed) : APPS.filter(a => a.cat === cat)).map(a => (
            <div key={a.slug} className="appcard">
              <div className="appcard__icon" style={{ background: `linear-gradient(135deg, ${a.color}, oklch(from ${a.color} calc(l - 0.1) c calc(h + 30)))` }}>
                {a.name.slice(0,2)}
              </div>
              <div className="appcard__name">{a.name}</div>
              <div className="appcard__cat muted">{catLabel(a.cat)} · v{a.ver}</div>
              <button className={`btn btn--sm ${a.installed?"":"btn--primary"}`} style={{marginTop:"auto"}}>
                {a.installed ? "Open" : "Install"}
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ═══════════════ FILE STATION ═══════════════
function FileStation() {
  const [view, setView] = React.useState("grid");
  const [sel, setSel] = React.useState(null);
  return (
    <div className="app-files">
      <div className="files-toolbar">
        <div className="files-path mono"><span className="muted">/</span> family-media <Icon name="chev" size={11}/> Photos</div>
        <div className="row gap-8" style={{marginLeft:"auto"}}>
          <div className="seg">
            <button className={view==="grid"?"is-on":""} onClick={() => setView("grid")}><Icon name="grid" size={12}/></button>
            <button className={view==="list"?"is-on":""} onClick={() => setView("list")}><Icon name="log" size={12}/></button>
          </div>
        </div>
      </div>
      <div className="files-split">
        <div className="files-tree">
          <div className="files-tree__group">VOLUMES</div>
          <div className="files-tree__item is-on"><Icon name="storage" size={12}/>family-media</div>
          <div className="files-tree__item"><Icon name="storage" size={12}/>pascal/docs</div>
          <div className="files-tree__item"><Icon name="storage" size={12}/>pascal/photos</div>
          <div className="files-tree__item"><Icon name="storage" size={12}/>backups</div>
          <div className="files-tree__group">SHARED</div>
          <div className="files-tree__item"><Icon name="user" size={12}/>family</div>
          <div className="files-tree__item"><Icon name="user" size={12}/>pascal</div>
        </div>
        <div className={`files-${view}`}>
          {FILES.map(f => (
            <div key={f.name} className={`file-item ${sel===f.name?"is-on":""}`} onClick={() => setSel(f.name)}>
              <div className="file-item__icon" data-kind={f.kind}>
                <Icon name={f.kind==="folder"?"folder":f.kind==="image"?"image":f.kind==="video"?"video":"doc"} size={view==="grid"?28:14}/>
              </div>
              <div className="file-item__name">{f.name}</div>
              {view==="list" && <>
                <div className="muted mono">{f.size ? fmtBytes(f.size) : "—"}</div>
                <div className="muted">{f.mod}</div>
              </>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ═══════════════ VIRTUALIZATION ═══════════════
function Virtualization() {
  const [sel, setSel] = React.useState(VMS[0].name);
  const cur = VMS.find(v => v.name === sel);
  return (
    <div className="app-vm">
      <div className="vm-list">
        {VMS.map(v => (
          <div key={v.name} className={`vm-item ${sel===v.name?"is-on":""}`} onClick={() => setSel(v.name)}>
            <span className={`vm-item__dot vm-item__dot--${v.state==="Running"?"on":"off"}`}/>
            <div className="stretch">
              <div style={{color:"var(--fg-0)"}}>{v.name}</div>
              <div className="muted mono" style={{fontSize:10}}>{v.os}</div>
            </div>
            <span className="muted mono" style={{fontSize:10}}>{v.cpu}c · {v.ram/1024}G</span>
          </div>
        ))}
      </div>
      <div className="vm-detail">
        <div className="vm-screen" data-state={cur.state}>
          {cur.state === "Running" ? (
            <div className="vm-screen__os mono">
              <div style={{opacity:0.6,fontSize:10,marginBottom:8}}>{cur.os}</div>
              <div style={{fontSize:14,color:"var(--fg-0)"}}>{cur.name}</div>
              <div className="vm-screen__cursor"/>
            </div>
          ) : (
            <div className="vm-screen__os" style={{opacity:0.4}}>● Powered off</div>
          )}
        </div>
        <div className="vm-controls">
          <button className="btn btn--sm"><Icon name={cur.state==="Running"?"pause":"play"} size={11}/>{cur.state==="Running"?"Pause":"Start"}</button>
          <button className="btn btn--sm"><Icon name="power" size={11}/>{cur.state==="Running"?"Shutdown":"Off"}</button>
          <button className="btn btn--sm"><Icon name="refresh" size={11}/>Restart</button>
          <button className="btn btn--sm" style={{marginLeft:"auto"}}><Icon name="terminal" size={11}/>Console</button>
        </div>
        <dl className="kv">
          <dt>OS</dt><dd>{cur.os}</dd>
          <dt>vCPU</dt><dd>{cur.cpu}</dd>
          <dt>Memory</dt><dd>{cur.ram} MiB</dd>
          <dt>IP</dt><dd>{cur.ip || "—"}</dd>
          <dt>State</dt><dd>{cur.state}</dd>
        </dl>
      </div>
    </div>
  );
}

// ═══════════════ CONTROL PANEL ═══════════════
function ControlPanel() {
  const sections = [
    { name: "Network", icon: "net", items: ["Interfaces","Routes","DNS","Firewall"] },
    { name: "Identity", icon: "user", items: ["Users","Groups","SSO","API tokens"] },
    { name: "Security", icon: "shield", items: ["Encryption","Certificates","2FA","Audit"] },
    { name: "Hardware", icon: "cpu", items: ["CPU & Power","Fans","UPS","Sensors"] },
    { name: "Notifications", icon: "bell", items: ["Channels","Rules","Webhooks"] },
    { name: "Backup", icon: "download", items: ["Replication","Cloud sync","Schedule"] },
  ];
  return (
    <div className="app-control">
      {sections.map(s => (
        <div key={s.name} className="control-card">
          <div className="control-card__head">
            <div className="control-card__icon"><Icon name={s.icon} size={16}/></div>
            <div className="control-card__name">{s.name}</div>
          </div>
          <ul className="control-card__items">
            {s.items.map(i => <li key={i}>{i}<Icon name="chev" size={12} style={{opacity:0.4}}/></li>)}
          </ul>
        </div>
      ))}
    </div>
  );
}

// ═══════════════ RESOURCE MONITOR (widget-sized) ═══════════════
function ResourceMonitor({ accent }) {
  const cpu = useSeries(40, 0.4, 0.15, 7);
  const mem = useSeries(40, 0.62, 0.05, 11);
  const net = useSeries(40, 0.3, 0.18, 17);
  const dsk = useSeries(40, 0.45, 0.12, 23);
  return (
    <div className="app-monitor">
      <div className="mon-row">
        <div className="mon-row__lbl"><Icon name="cpu" size={11}/> CPU</div>
        <div className="mon-row__spark"><Spark data={cpu} color={accent} h={28}/></div>
        <div className="mon-row__num mono">{Math.round(cpu[cpu.length-1]*100)}%</div>
      </div>
      <div className="mon-row">
        <div className="mon-row__lbl"><Icon name="ram" size={11}/> Mem</div>
        <div className="mon-row__spark"><Spark data={mem} color={accent} h={28}/></div>
        <div className="mon-row__num mono">{Math.round(mem[mem.length-1]*100)}%</div>
      </div>
      <div className="mon-row">
        <div className="mon-row__lbl"><Icon name="net" size={11}/> Net</div>
        <div className="mon-row__spark"><Spark data={net} color={accent} h={28}/></div>
        <div className="mon-row__num mono">{(net[net.length-1]*1.2).toFixed(1)}G</div>
      </div>
      <div className="mon-row">
        <div className="mon-row__lbl"><Icon name="storage" size={11}/> Disk</div>
        <div className="mon-row__spark"><Spark data={dsk} color={accent} h={28}/></div>
        <div className="mon-row__num mono">{Math.round(dsk[dsk.length-1]*100)}%</div>
      </div>
    </div>
  );
}

// ═══════════════ NOTIFICATIONS ═══════════════
function Notifications() {
  return (
    <div className="app-notifs">
      {ACTIVITY.map((a,i) => (
        <div key={i} className="notif-item">
          <span className={`sdot sdot--${a.tone}`}/>
          <div className="stretch">{a.text}</div>
          <span className="muted mono" style={{fontSize:10}}>{a.t}</span>
        </div>
      ))}
    </div>
  );
}

// ═══════════════ TERMINAL ═══════════════
function TerminalApp() {
  return (
    <div className="app-term">
      <div className="term-line"><span className="term-prompt">novanas:~$</span> zpool status</div>
      <div className="term-out">  pool: fast<br/> state: ONLINE<br/>  scan: scrub repaired 0B in 2h14m</div>
      <div className="term-line"><span className="term-prompt">novanas:~$</span> zfs list -t snapshot | head -5</div>
      <div className="term-out">family-media@auto-14:58    1.4G<br/>pascal/docs@daily-04:00   240M<br/>vm-disks@pre-update       4.1G<br/>backups@weekly-W17        18.0G<br/>pascal/photos@trip         3.2G</div>
      <div className="term-line"><span className="term-prompt">novanas:~$</span> <span className="term-cursor">_</span></div>
    </div>
  );
}

window.StorageManager = StorageManager;
window.AppCenter = AppCenter;
window.FileStation = FileStation;
window.Virtualization = Virtualization;
window.ControlPanel = ControlPanel;
window.ResourceMonitor = ResourceMonitor;
window.Notifications = Notifications;
window.TerminalApp = TerminalApp;
