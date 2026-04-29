/* globals React, Icon, Spark, useSeries, Ring, useWindowManager, useWindowDrag,
   POOLS, DISKS, ACTIVITY, NOTIFICATIONS, ALERTS, JOBS, fmtBytes, fmtPct,
   StorageManager, Replication, ScrubPolicies, NetworkApp, Shares, Identity,
   WorkloadsApp, Virt2, PackageCenter, Alerts, Logs, Audit, JobsApp, NotificationCenter, SystemApp, FileStationApp, TerminalApp2,
   ResourceMonitor */

const APP_REGISTRY = {
  // Core
  storage:    { title: "Storage Manager",    icon: "storage",  comp: () => <StorageManager/>,    w: 920, h: 600, tier: "core" },
  replication:{ title: "Replication",        icon: "refresh",  comp: () => <Replication/>,       w: 880, h: 540, tier: "core" },
  scrub:      { title: "Scrub Policies",     icon: "shield",   comp: () => <ScrubPolicies/>,     w: 760, h: 460, tier: "core" },
  network:    { title: "Network",            icon: "net",      comp: () => <NetworkApp/>,        w: 820, h: 520, tier: "core" },
  identity:   { title: "Identity",           icon: "user",     comp: () => <Identity/>,          w: 860, h: 540, tier: "core" },
  alerts:     { title: "Alerts",             icon: "bell",     comp: () => <Alerts/>,            w: 880, h: 540, tier: "core" },
  logs:       { title: "Logs",               icon: "log",      comp: () => <Logs/>,              w: 880, h: 540, tier: "core" },
  audit:      { title: "Audit Log",          icon: "shield",   comp: () => <Audit/>,             w: 820, h: 520, tier: "core" },
  jobs:       { title: "Jobs",               icon: "bolt",     comp: () => <JobsApp/>,           w: 800, h: 480, tier: "core" },
  notifs:     { title: "Notification Center", icon: "bell",    comp: () => <NotificationCenter/>, w: 820, h: 520, tier: "core" },
  system:     { title: "System",             icon: "cpu",      comp: () => <SystemApp/>,         w: 720, h: 540, tier: "core" },
  files:      { title: "File Station",       icon: "files",    comp: () => <FileStationApp/>,    w: 800, h: 520, tier: "core" },
  terminal:   { title: "Terminal",           icon: "terminal", comp: () => <TerminalApp2/>,      w: 600, h: 380, tier: "core" },
  monitor:    { title: "Resource Monitor",   icon: "monitor",  comp: () => <ResourceMonitor accent="var(--accent)"/>, w: 380, h: 280, tier: "core" },
  // Tier 1 — first-party installable
  shares:     { title: "Protocol Shares",    icon: "net",      comp: () => <Shares/>,            w: 880, h: 520, tier: "tier1" },
  workloads:  { title: "Apps Center",        icon: "apps",     comp: () => <WorkloadsApp/>,      w: 920, h: 580, tier: "tier1" },
  vms:        { title: "Virtualization",     icon: "vm",       comp: () => <Virt2/>,             w: 860, h: 560, tier: "tier1" },
  // Tier 2 — federated marketplace
  packages:   { title: "Package Center",     icon: "apps",     comp: () => <PackageCenter/>,     w: 940, h: 600, tier: "tier2" },
};

// ───────── Window chrome ─────────
function WindowFrame({ win, mgr, surfaceRef, children }) {
  const reg = APP_REGISTRY[win.app];
  const { onTitleDown, onResizeDown } = useWindowDrag(win, mgr, surfaceRef);
  const style = win.max
    ? { left: 0, top: 28, right: 0, bottom: 56, width: "auto", height: "auto", zIndex: win.z }
    : { left: win.x, top: win.y, width: win.w, height: win.h, zIndex: win.z };
  if (win.min) return null;
  return (
    <div className="win" style={style} onMouseDown={() => mgr.focus(win.id)}>
      <div className="win__bar" onMouseDown={onTitleDown} onDoubleClick={() => mgr.toggleMax(win.id)}>
        <div className="win__title">
          <Icon name={reg.icon} size={12}/>
          <span>{reg.title}</span>
        </div>
        <div className="win__btns">
          <button onClick={() => mgr.toggleMin(win.id)} title="Minimize"><Icon name="min" size={10}/></button>
          <button onClick={() => mgr.toggleMax(win.id)} title="Maximize"><Icon name="max" size={9}/></button>
          <button className="win__close" onClick={() => mgr.close(win.id)} title="Close"><Icon name="close" size={10}/></button>
        </div>
      </div>
      <div className="win__body">{children}</div>
      {!win.max && <div className="win__resize" onMouseDown={onResizeDown}/>}
    </div>
  );
}

// ───────── Top bar ─────────
function TopBar({ onLauncher, onOpen, mgr, variant, onPalette, onBell, onUserMenu, alertsOpen }) {
  const time = useClock();
  const unread = NOTIFICATIONS.filter(n => !n.read).length;
  return (
    <div className="topbar">
      <button className="topbar__menu" onClick={onLauncher} title="Main Menu">
        <Icon name="grid" size={16}/>
      </button>
      <div className="topbar__brand">
        <div className="topbar__logo">N</div>
        <div className="topbar__name">NovaNAS</div>
      </div>
      <div className="topbar__divider"/>
      <div className="topbar__running">
        {mgr.wins.map(w => {
          const reg = APP_REGISTRY[w.app];
          return (
            <button key={w.id}
              className={`tb-task ${!w.min ? "is-on" : ""}`}
              onClick={() => w.min ? mgr.focus(w.id) : mgr.toggleMin(w.id)}
              title={reg.title}>
              <Icon name={reg.icon} size={13}/>
              <span>{reg.title}</span>
            </button>
          );
        })}
      </div>
      <div className="topbar__spacer"/>
      <button className="topbar__search" onClick={onPalette}>
        <Icon name="search" size={12}/>
        <span style={{flex:1,textAlign:"left",color:"var(--fg-3)",fontSize:11}}>Search apps, files, settings…</span>
        <kbd>⌘K</kbd>
      </button>
      {ALERTS.filter(a=>a.severity==="critical").length > 0 && (
        <button className="topbar__icon" onClick={()=>onOpen("alerts")} title="Critical alerts" style={{color:"var(--err)"}}>
          <Icon name="warning" size={15}/>
          <span className="badge" style={{background:"var(--err)"}}/>
        </button>
      )}
      <button className="topbar__icon" onClick={onBell} title="Notifications">
        <Icon name="bell" size={15}/>
        {unread > 0 && <span className="badge"/>}
      </button>
      <button className="topbar__icon" onClick={onUserMenu} title="User"><Icon name="user" size={15}/></button>
      <div className="topbar__clock mono">{time}</div>
    </div>
  );
}

function useClock() {
  const [t, setT] = React.useState(() => {
    const d = new Date();
    return `${String(d.getHours()).padStart(2,"0")}:${String(d.getMinutes()).padStart(2,"0")}`;
  });
  React.useEffect(() => {
    const tick = () => {
      const d = new Date();
      setT(`${String(d.getHours()).padStart(2,"0")}:${String(d.getMinutes()).padStart(2,"0")}`);
    };
    const id = setInterval(tick, 30000);
    return () => clearInterval(id);
  }, []);
  return t;
}

// ───────── Notification bell drawer ─────────
function BellDrawer({ open, onClose, onOpen }) {
  if (!open) return null;
  return (
    <>
      <div style={{position:"absolute",inset:0,zIndex:7900}} onClick={onClose}/>
      <div className="bell-drawer">
        <div className="bell-drawer__head">
          <Icon name="bell" size={12}/>
          <span>Notifications</span>
          <span className="muted" style={{fontSize:10,fontWeight:400,marginLeft:"auto"}}>{NOTIFICATIONS.filter(n=>!n.read).length} unread</span>
        </div>
        <div className="bell-drawer__list">
          {NOTIFICATIONS.slice(0,8).map(n => (
            <div key={n.id} className={`bell-item ${!n.read?"bell-item--unread":""}`}>
              <span className={`sdot sdot--${n.sev==="error"?"err":n.sev==="warn"?"warn":n.sev==="ok"?"ok":"info"}`}/>
              <div>
                <div className="bell-item__title">{n.title}</div>
                <div className="bell-item__sub">{n.src} · {n.actor}</div>
              </div>
              <span className="muted mono" style={{fontSize:10}}>{n.at}</span>
            </div>
          ))}
        </div>
        <div style={{padding:"8px 12px",borderTop:"1px solid var(--line)",display:"flex",gap:8}}>
          <button className="btn btn--sm" style={{flex:1}} onClick={()=>{onOpen("notifs"); onClose();}}>Open Center</button>
          <button className="btn btn--sm">Mark all read</button>
        </div>
      </div>
    </>
  );
}

// ───────── User menu ─────────
function UserMenu({ open, onClose, onOpen }) {
  if (!open) return null;
  return (
    <>
      <div style={{position:"absolute",inset:0,zIndex:7900}} onClick={onClose}/>
      <div className="user-menu">
        <div className="user-menu__head">
          <div className="avatar" style={{width:34,height:34,fontSize:11,"--avh":260}}>PA</div>
          <div>
            <div style={{color:"var(--fg-0)",fontSize:12,fontWeight:500}}>pascal</div>
            <div className="muted" style={{fontSize:10}}>nova-admin</div>
          </div>
        </div>
        <button className="user-menu__btn" onClick={()=>{onOpen("identity"); onClose();}}><Icon name="user" size={11}/>My account</button>
        <button className="user-menu__btn" onClick={()=>{onOpen("identity"); onClose();}}><Icon name="shield" size={11}/>Sessions & login history</button>
        <button className="user-menu__btn" onClick={()=>{onOpen("system"); onClose();}}><Icon name="cpu" size={11}/>System</button>
        <div style={{height:1,background:"var(--line)",margin:"4px 0"}}/>
        <button className="user-menu__btn"><Icon name="power" size={11}/>Sign out</button>
        <button className="user-menu__btn"><Icon name="refresh" size={11}/>Restart</button>
        <button className="user-menu__btn" style={{color:"var(--err)"}}><Icon name="power" size={11}/>Power off</button>
      </div>
    </>
  );
}

// ───────── Cmd-K palette ─────────
function Palette({ open, onClose, onOpen }) {
  const [q, setQ] = React.useState("");
  const [idx, setIdx] = React.useState(0);
  const inputRef = React.useRef(null);
  React.useEffect(() => {
    if (open) { setQ(""); setIdx(0); setTimeout(()=>inputRef.current?.focus(),20); }
  }, [open]);
  if (!open) return null;
  const apps = Object.entries(APP_REGISTRY).map(([k,v]) => ({type:"app", key:k, ...v, label: v.title}));
  const filtered = apps.filter(a => !q || a.label.toLowerCase().includes(q.toLowerCase()));
  const pick = (a) => { onOpen(a.key); onClose(); };
  return (
    <div className="palette-bg" onClick={onClose}>
      <div className="palette" onClick={e=>e.stopPropagation()} onKeyDown={e=>{
        if (e.key==="ArrowDown") { e.preventDefault(); setIdx(i => Math.min(filtered.length-1, i+1)); }
        if (e.key==="ArrowUp")   { e.preventDefault(); setIdx(i => Math.max(0, i-1)); }
        if (e.key==="Enter" && filtered[idx]) { e.preventDefault(); pick(filtered[idx]); }
        if (e.key==="Escape") onClose();
      }}>
        <div className="palette__input">
          <Icon name="search" size={14}/>
          <input ref={inputRef} placeholder="Search apps, settings, actions…" value={q} onChange={e=>{setQ(e.target.value); setIdx(0);}}/>
          <kbd>esc</kbd>
        </div>
        <div className="palette__list">
          <div className="palette__item palette__item--cat">Apps</div>
          {filtered.slice(0,12).map((a,i) => (
            <div key={a.key} className={`palette__item ${i===idx?"is-on":""}`} onClick={()=>pick(a)} onMouseEnter={()=>setIdx(i)}>
              <Icon name={a.icon} size={13}/>
              <span style={{flex:1}}>{a.label}</span>
              <span className="muted mono" style={{fontSize:9}}>{a.tier}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ───────── Dock ─────────
function Dock({ onOpen }) {
  const items = ["storage","workloads","vms","files","alerts","packages","terminal"];
  return (
    <div className="dock">
      {items.map(k => {
        const reg = APP_REGISTRY[k];
        return (
          <button key={k} className="dock__btn" onClick={() => onOpen(k)} title={reg.title}>
            <Icon name={reg.icon} size={20}/>
            <span className="dock__lbl">{reg.title}</span>
          </button>
        );
      })}
    </div>
  );
}

// ───────── Main Menu launcher (full-screen) — three-tier ─────────
function Launcher({ open, onClose, onOpen }) {
  if (!open) return null;
  const groups = {
    "Core": Object.entries(APP_REGISTRY).filter(([_,r]) => r.tier === "core"),
    "Installed (first-party)": Object.entries(APP_REGISTRY).filter(([_,r]) => r.tier === "tier1"),
    "Marketplace": Object.entries(APP_REGISTRY).filter(([_,r]) => r.tier === "tier2"),
  };
  return (
    <div className="launcher" onClick={onClose}>
      <div className="launcher__inner" onClick={e => e.stopPropagation()} style={{maxWidth:920}}>
        <div className="launcher__title">All Apps</div>
        {Object.entries(groups).map(([g, items]) => (
          <div key={g} style={{marginBottom:18}}>
            <div className="vlist__title" style={{padding:"0 4px 8px"}}>{g}</div>
            <div className="launcher__grid">
              {items.map(([k, reg]) => (
                <button key={k} className="launcher__item" onClick={() => { onOpen(k); onClose(); }}>
                  <div className="launcher__icon"><Icon name={reg.icon} size={26}/></div>
                  <div className="launcher__name">{reg.title}</div>
                </button>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ───────── Desktop widgets ─────────
function HealthWidget({ onOpen }) {
  const usedTotal = POOLS.reduce((m, p) => m + p.used, 0);
  const totalTotal = POOLS.reduce((m, p) => m + p.total, 0);
  const pct = usedTotal / totalTotal;
  const critAlerts = ALERTS.filter(a => a.severity === "critical").length;
  const warnAlerts = ALERTS.filter(a => a.severity === "warning").length;
  return (
    <div className="widget widget--health">
      <div className="widget__head">
        <div className="widget__title">System Health</div>
        <span className={`pill pill--${critAlerts>0?"err":warnAlerts>0?"warn":"ok"}`}><span className="dot"/>{critAlerts>0?"Action needed":warnAlerts>0?"Warnings":"Healthy"}</span>
      </div>
      <div className="widget__body widget__body--ring">
        <Ring value={pct} size={120} stroke={9} label={fmtPct(pct)} sub="capacity"/>
        <div className="widget__stats">
          <div onClick={()=>onOpen("storage")} style={{cursor:"pointer"}}><div className="muted">Pools</div><div className="mono fg0">{POOLS.length}</div></div>
          <div onClick={()=>onOpen("storage")} style={{cursor:"pointer"}}><div className="muted">Disks</div><div className="mono fg0">{DISKS.filter(d => d.state !== "EMPTY").length}/24</div></div>
          <div><div className="muted">Uptime</div><div className="mono fg0">12d 4h</div></div>
          <div onClick={()=>onOpen("alerts")} style={{cursor:"pointer"}}>
            <div className="muted">Alerts</div>
            <div className="mono" style={{color: critAlerts>0?"var(--err)":warnAlerts>0?"var(--warn)":"var(--fg-0)"}}>
              {critAlerts + warnAlerts}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function MonitorWidget() {
  return (
    <div className="widget widget--monitor">
      <div className="widget__head">
        <div className="widget__title">Resource Monitor</div>
        <span className="muted mono" style={{fontSize:10}}>live · 5s</span>
      </div>
      <div className="widget__body">
        <ResourceMonitor accent="var(--accent)"/>
      </div>
    </div>
  );
}

function ActivityWidget({ onOpen }) {
  return (
    <div className="widget widget--activity">
      <div className="widget__head">
        <div className="widget__title">Recent Activity</div>
        <button className="btn btn--sm" onClick={()=>onOpen("audit")}>All</button>
      </div>
      <div className="widget__body widget__body--flush">
        {ACTIVITY.slice(0, 5).map((a,i) => (
          <div key={i} className="activity-row">
            <span className={`sdot sdot--${a.tone}`}/>
            <div className="stretch">{a.text}</div>
            <span className="muted mono" style={{fontSize:10}}>{a.t}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function PoolWidget({ onOpen }) {
  return (
    <div className="widget widget--pools">
      <div className="widget__head">
        <div className="widget__title">Storage Pools</div>
        <button className="btn btn--sm" onClick={()=>onOpen("storage")}>Manage</button>
      </div>
      <div className="widget__body widget__body--flush">
        {POOLS.map(p => {
          const pct = p.used / p.total;
          return (
            <div key={p.name} className="pool-row" onClick={()=>onOpen("storage")} style={{cursor:"pointer"}}>
              <div className="pool-row__name">
                <span className={`tier-mark tier-mark--${p.tier}`}/>
                <span>{p.name}</span>
                <span className="muted mono" style={{fontSize:10}}>{p.protection}</span>
              </div>
              <div className="pool-row__bar"><div style={{width:`${pct*100}%`}}/></div>
              <span className="mono" style={{fontSize:11,color:"var(--fg-2)",minWidth:64,textAlign:"right"}}>{fmtPct(pct)}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function JobsWidget({ onOpen }) {
  const running = JOBS.filter(j => j.state === "running");
  return (
    <div className="widget">
      <div className="widget__head">
        <div className="widget__title">Active Jobs</div>
        <button className="btn btn--sm" onClick={()=>onOpen("jobs")}>All</button>
      </div>
      <div className="widget__body widget__body--flush">
        {running.length === 0 ? <div className="empty-hint">No jobs running</div> : running.map(j => (
          <div key={j.id} className="timeline" onClick={()=>onOpen("jobs")} style={{cursor:"pointer",padding:"6px 12px"}}>
            <span className="mono muted" style={{fontSize:10}}>{j.kind.split(".")[0]}</span>
            <div className="cap__bar"><div style={{width:`${j.pct*100}%`}}/></div>
            <span className="mono" style={{fontSize:11}}>{j.eta}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ───────── Desktop (combines everything) ─────────
function Desktop({ variant }) {
  const surfaceRef = React.useRef(null);
  const mgr = useWindowManager([
    { id: "w1", app: "storage",   x: 30,  y: 60,  w: 880, h: 540, z: 1 },
    { id: "w2", app: "alerts",    x: 280, y: 130, w: 820, h: 460, z: 2 },
  ]);
  const [launcher, setLauncher] = React.useState(false);
  const [palette, setPalette] = React.useState(false);
  const [bell, setBell] = React.useState(false);
  const [userMenu, setUserMenu] = React.useState(false);

  const onOpen = React.useCallback((app) => {
    const reg = APP_REGISTRY[app];
    if (!reg) return;
    const sw = surfaceRef.current?.clientWidth || 1280;
    const sh = surfaceRef.current?.clientHeight || 720;
    const existing = mgr.wins.find(w => w.app === app);
    if (existing) { mgr.focus(existing.id); if (existing.min) mgr.toggleMin(existing.id); return; }
    const x = Math.max(20, Math.min(sw - reg.w - 20, (sw - reg.w) / 2 + (Math.random() - 0.5) * 100));
    const y = Math.max(50, Math.min(sh - reg.h - 80, (sh - reg.h) / 2 + (Math.random() - 0.5) * 80));
    mgr.open({ id: `w-${app}-${Date.now()}`, app, x, y, w: reg.w, h: reg.h });
  }, [mgr]);

  // Cmd-K
  React.useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPalette(p => !p);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className={`os os--${variant}`} ref={surfaceRef}>
      <div className="wallpaper"/>
      <TopBar
        mgr={mgr}
        onLauncher={() => setLauncher(s => !s)}
        onOpen={onOpen}
        variant={variant}
        onPalette={() => setPalette(true)}
        onBell={() => setBell(b => !b)}
        onUserMenu={() => setUserMenu(u => !u)}
      />

      <div className="desktop-widgets">
        <HealthWidget onOpen={onOpen}/>
        <PoolWidget onOpen={onOpen}/>
        <ActivityWidget onOpen={onOpen}/>
        <JobsWidget onOpen={onOpen}/>
      </div>

      <div className="windows-layer">
        {mgr.wins.map(w => {
          const reg = APP_REGISTRY[w.app];
          if (!reg) return null;
          const Comp = reg.comp;
          return (
            <WindowFrame key={w.id} win={w} mgr={mgr} surfaceRef={surfaceRef}>
              <Comp/>
            </WindowFrame>
          );
        })}
      </div>

      <Dock onOpen={onOpen}/>
      <Launcher open={launcher} onClose={() => setLauncher(false)} onOpen={onOpen}/>
      <Palette open={palette} onClose={() => setPalette(false)} onOpen={onOpen}/>
      <BellDrawer open={bell} onClose={() => setBell(false)} onOpen={onOpen}/>
      <UserMenu open={userMenu} onClose={() => setUserMenu(false)} onOpen={onOpen}/>
    </div>
  );
}

window.Desktop = Desktop;
