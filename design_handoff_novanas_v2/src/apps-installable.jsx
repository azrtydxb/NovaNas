/* globals React, Icon, Spark, useSeries,
   APPS, WORKLOADS, VMS, VM_TEMPLATES, VM_SNAPSHOTS, PLUGINS, MARKETPLACE_PLUGINS, MARKETPLACES,
   ALERTS, ALERT_SILENCES, ALERT_RECEIVERS, LOG_LABELS, LOG_LINES, AUDIT, JOBS, NOTIFICATIONS,
   SYSTEM_INFO, SYSTEM_UPDATE, SMTP_CONFIG, FILES, ACTIVITY,
   fmtBytes, fmtPct */

const Sect2 = ({title, action, children}) => (
  <div className="sect">
    <div className="sect__head"><div className="sect__title">{title}</div>{action}</div>
    <div className="sect__body">{children}</div>
  </div>
);
const TBar2 = ({children}) => <div className="tbar">{children}</div>;

// ═════════ WORKLOADS / APPS (Helm releases) ═════════
function WorkloadsApp() {
  const [tab, setTab] = React.useState("releases");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["releases","catalog","events"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="releases" && <WorkloadsList/>}
        {tab==="catalog" && <WorkloadsCatalog/>}
        {tab==="events" && <WorkloadsEvents/>}
      </div>
    </div>
  );
}

function WorkloadsList() {
  const [sel, setSel] = React.useState("immich");
  const cur = WORKLOADS.find(w => w.release === sel);
  return (
    <div style={{display:"grid",gridTemplateColumns:"1fr 320px",height:"100%"}}>
      <div style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <button className="btn btn--primary"><Icon name="plus" size={11}/>Install chart</button>
          <button className="btn">Upgrade all</button>
        </TBar2>
        <table className="tbl">
          <thead><tr><th>Release</th><th>Chart</th><th>Version</th><th>Namespace</th><th className="num">Pods</th><th className="num">CPU</th><th className="num">Memory</th><th>Status</th></tr></thead>
          <tbody>{WORKLOADS.map(w => (
            <tr key={w.release} onClick={()=>setSel(w.release)} className={sel===w.release?"is-on":""}>
              <td><Icon name="apps" size={12} style={{verticalAlign:"-2px",marginRight:6,opacity:0.6}}/>{w.release}</td>
              <td className="muted mono" style={{fontSize:11}}>{w.chart}</td>
              <td className="mono" style={{fontSize:11}}>{w.version}</td>
              <td className="muted mono" style={{fontSize:11}}>{w.ns}</td>
              <td className="num mono">{w.pods}</td>
              <td className="num mono">{w.cpu}</td>
              <td className="num mono">{w.mem}</td>
              <td><span className={`pill pill--${w.status==="Deployed"?"ok":w.status==="Pending"?"info":"err"}`}><span className="dot"/>{w.status}</span></td>
            </tr>
          ))}</tbody>
        </table>
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div><div className="muted mono" style={{fontSize:10}}>RELEASE</div><div className="side-detail__title">{cur.release}</div></div>
          </div>
          <Sect2 title="Chart">
            <dl className="kv">
              <dt>Chart</dt><dd className="mono">{cur.chart}</dd>
              <dt>Version</dt><dd className="mono">{cur.version}</dd>
              <dt>Namespace</dt><dd className="mono">{cur.ns}</dd>
              <dt>Updated</dt><dd>{cur.updated}</dd>
            </dl>
          </Sect2>
          <Sect2 title="Resources">
            <dl className="kv">
              <dt>Pods</dt><dd className="mono">{cur.pods}</dd>
              <dt>CPU</dt><dd className="mono">{cur.cpu} cores</dd>
              <dt>Memory</dt><dd className="mono">{cur.mem}</dd>
            </dl>
          </Sect2>
          <Sect2 title="Pods">
            <table className="tbl tbl--compact">
              <tbody>
                <tr><td className="mono">{cur.release}-app-0</td><td><span className="sdot sdot--ok"/> Running</td><td className="muted">2d</td></tr>
                <tr><td className="mono">{cur.release}-worker-0</td><td><span className="sdot sdot--ok"/> Running</td><td className="muted">2d</td></tr>
              </tbody>
            </table>
          </Sect2>
          <div className="row gap-8" style={{padding:"10px 12px",borderTop:"1px solid var(--line)",flexWrap:"wrap"}}>
            <button className="btn btn--sm btn--primary">Upgrade</button>
            <button className="btn btn--sm">Edit values</button>
            <button className="btn btn--sm">Rollback…</button>
            <button className="btn btn--sm btn--danger" style={{marginLeft:"auto"}}>Uninstall</button>
          </div>
        </div>
      )}
    </div>
  );
}

function WorkloadsCatalog() {
  return (
    <div style={{padding:14}}>
      <TBar2>
        <input className="input" placeholder="Search catalog…" style={{width:240}}/>
        <span className="muted" style={{fontSize:11,marginLeft:"auto"}}>Index updated 12 min ago</span>
        <button className="btn btn--sm">Refresh</button>
      </TBar2>
      <div className="appcards">
        {APPS.filter(a=>!a.installed).slice(0,9).map(a => (
          <div key={a.slug} className="appcard">
            <div className="appcard__icon" style={{background:`linear-gradient(135deg, ${a.color}, oklch(from ${a.color} calc(l - 0.1) c calc(h + 30)))`}}>{a.name.slice(0,2)}</div>
            <div className="appcard__name">{a.name}</div>
            <div className="appcard__cat muted">{a.cat.charAt(0).toUpperCase()+a.cat.slice(1)} · v{a.ver}</div>
            <button className="btn btn--sm btn--primary" style={{marginTop:"auto"}}>Install</button>
          </div>
        ))}
      </div>
    </div>
  );
}

function WorkloadsEvents() {
  const events = [
    { t: "14:02:38", kind: "Normal", reason: "Started",  obj: "pod/immich-app-0",        msg: "Started container app" },
    { t: "14:01:55", kind: "Normal", reason: "Pulled",   obj: "pod/immich-app-0",        msg: "Image \"ghcr.io/immich-app/immich-server:1.140.0\" already present" },
    { t: "13:58:12", kind: "Normal", reason: "Scheduled", obj: "pod/plex-1",             msg: "Successfully assigned default/plex-1 to nas.lan" },
    { t: "13:42:01", kind: "Warning", reason: "BackOff", obj: "pod/jellyfin-2",          msg: "Back-off restarting failed container" },
    { t: "13:30:00", kind: "Normal", reason: "Upgrade",  obj: "release/grafana",         msg: "Helm upgrade succeeded: grafana-7.0.18 → 7.0.21" },
  ];
  return (
    <div style={{padding:14}}>
      <table className="tbl">
        <thead><tr><th>Time</th><th>Kind</th><th>Reason</th><th>Object</th><th>Message</th></tr></thead>
        <tbody>{events.map((e,i) => (
          <tr key={i}>
            <td className="muted mono" style={{fontSize:11}}>{e.t}</td>
            <td>{e.kind==="Warning"?<span className="pill pill--warn"><span className="dot"/>{e.kind}</span>:<span className="pill"><span className="dot"/>{e.kind}</span>}</td>
            <td className="mono" style={{fontSize:11}}>{e.reason}</td>
            <td className="mono muted" style={{fontSize:11}}>{e.obj}</td>
            <td className="muted">{e.msg}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ VIRTUALIZATION ═════════
function Virt2() {
  const [tab, setTab] = React.useState("vms");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["vms","templates","snapshots"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="vms" && <VMList/>}
        {tab==="templates" && <VMTemplates/>}
        {tab==="snapshots" && <VMSnapshots/>}
      </div>
    </div>
  );
}

function VMList() {
  const [sel, setSel] = React.useState(VMS[0].name);
  const cur = VMS.find(v => v.name === sel);
  return (
    <div style={{display:"grid",gridTemplateColumns:"180px 1fr",height:"100%"}}>
      <div style={{borderRight:"1px solid var(--line)",overflow:"auto",padding:6}}>
        <div className="vlist__title" style={{padding:"4px 8px"}}>VIRTUAL MACHINES</div>
        {VMS.map(v => (
          <button key={v.name} className={`vlist__item ${sel===v.name?"is-on":""}`} onClick={()=>setSel(v.name)}>
            <span className={`sdot sdot--${v.state==="Running"?"ok":v.state==="Paused"?"warn":""}`}/>
            <div style={{flex:1,minWidth:0,textAlign:"left"}}>
              <div style={{color:"var(--fg-1)",fontSize:11}}>{v.name}</div>
              <div className="muted mono" style={{fontSize:9}}>{v.os}</div>
            </div>
          </button>
        ))}
      </div>
      <div style={{padding:14,overflow:"auto",display:"flex",flexDirection:"column",gap:12}}>
        <div className="row" style={{gap:8,flexWrap:"wrap"}}>
          <span className={`pill pill--${cur.state==="Running"?"ok":cur.state==="Paused"?"warn":""}`}><span className="dot"/>{cur.state}</span>
          {cur.ip && <span className="pill pill--info mono" style={{fontSize:10}}>{cur.ip}</span>}
          <span className="muted" style={{marginLeft:"auto",fontSize:11}}>uptime {cur.uptime}</span>
        </div>
        <div className="vm-console" style={{minHeight:160}}>
          {cur.state === "Running" ? (
            <>
              <div style={{opacity:0.6,fontSize:10,marginBottom:8}}>{cur.os} · console</div>
              <div>[ OK ] Started Network Time Synchronization.</div>
              <div>[ OK ] Reached target Multi-User System.</div>
              <div>[ OK ] kubernetes kubelet.service: Active</div>
              <div style={{marginTop:6}}>{cur.name} login: <span className="vm-console__cursor"/></div>
            </>
          ) : (
            <div style={{opacity:0.4,padding:30,textAlign:"center"}}>● Powered off</div>
          )}
        </div>
        <div className="row gap-8" style={{flexWrap:"wrap"}}>
          <button className="btn btn--sm btn--primary"><Icon name={cur.state==="Running"?"pause":"play"} size={11}/>{cur.state==="Running"?"Pause":"Start"}</button>
          <button className="btn btn--sm"><Icon name="power" size={11}/>{cur.state==="Running"?"Shutdown":"Boot"}</button>
          <button className="btn btn--sm"><Icon name="refresh" size={11}/>Restart</button>
          <button className="btn btn--sm">Migrate…</button>
          <button className="btn btn--sm">Snapshot</button>
          <button className="btn btn--sm" style={{marginLeft:"auto"}}><Icon name="terminal" size={11}/>Serial console</button>
        </div>
        <div className="row gap-12" style={{flexWrap:"wrap"}}>
          <div className="kpi"><div className="kpi__lbl">vCPU</div><div className="kpi__val mono">{cur.cpu}</div></div>
          <div className="kpi"><div className="kpi__lbl">Memory</div><div className="kpi__val mono">{(cur.ram/1024).toFixed(0)} <span className="muted">GiB</span></div></div>
          <div className="kpi"><div className="kpi__lbl">Disk</div><div className="kpi__val mono">{cur.disk}</div></div>
          <div className="kpi"><div className="kpi__lbl">MAC</div><div className="kpi__val mono" style={{fontSize:11}}>{cur.mac}</div></div>
        </div>
      </div>
    </div>
  );
}

function VMTemplates() {
  return (
    <div style={{padding:14}}>
      <TBar2><button className="btn btn--primary"><Icon name="plus" size={11}/>New template</button></TBar2>
      <table className="tbl">
        <thead><tr><th>Template</th><th>OS</th><th className="num">vCPU</th><th className="num">RAM</th><th className="num">Disk</th><th>Source</th></tr></thead>
        <tbody>{VM_TEMPLATES.map(t => (
          <tr key={t.name}>
            <td>{t.name}</td>
            <td className="muted">{t.os}</td>
            <td className="num mono">{t.cpu}</td>
            <td className="num mono">{(t.ram/1024).toFixed(0)} GiB</td>
            <td className="num mono">{t.disk} GiB</td>
            <td><span className="pill">{t.source}</span></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

function VMSnapshots() {
  return (
    <div style={{padding:14}}>
      <table className="tbl">
        <thead><tr><th>Snapshot</th><th>VM</th><th className="num">Size</th><th>Created</th><th></th></tr></thead>
        <tbody>{VM_SNAPSHOTS.map(s => (
          <tr key={s.name}>
            <td className="mono" style={{fontSize:11}}>{s.name}</td>
            <td>{s.vm}</td>
            <td className="num mono">{s.size}</td>
            <td className="muted">{s.t}</td>
            <td className="num"><button className="btn btn--sm">Restore</button></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ PACKAGE CENTER (federated marketplaces + plugins) ═════════
function PackageCenter() {
  const [tab, setTab] = React.useState("discover");
  const [installing, setInstalling] = React.useState(null);
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["discover","installed","marketplaces"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto",position:"relative"}}>
        {tab==="discover" && <Marketplace onInstall={p => setInstalling(p)}/>}
        {tab==="installed" && <InstalledPlugins/>}
        {tab==="marketplaces" && <MarketplacesTab/>}
        {installing && <InstallConsentDialog plugin={installing} onClose={()=>setInstalling(null)}/>}
      </div>
    </div>
  );
}

// Mirrors internal/plugins/manifest.go DisplayCategory enum (14 values).
// Keep in sync with the backend list — Aurora's category sidebar is
// driven by GET /api/v1/plugins/categories at runtime; this list is for
// the static prototype only.
const DISPLAY_CATEGORIES = [
  ["backup",        "Backup"],
  ["files",         "Files"],
  ["multimedia",    "Multimedia"],
  ["photos",        "Photos"],
  ["productivity",  "Productivity"],
  ["security",      "Security"],
  ["communication", "Communication"],
  ["home",          "Home"],
  ["developer",     "Developer"],
  ["network",       "Network"],
  ["storage",       "Storage"],
  ["surveillance",  "Surveillance"],
  ["utilities",     "Utilities"],
  ["observability", "Observability"],
];

function Marketplace({onInstall}) {
  const [src, setSrc] = React.useState("all");
  const [cat, setCat] = React.useState("all");
  let list = MARKETPLACE_PLUGINS;
  if (src !== "all") list = list.filter(p => p.source === src);
  if (cat !== "all") list = list.filter(p => p.displayCategory === cat);
  return (
    <div style={{display:"grid",gridTemplateColumns:"160px 1fr",height:"100%"}}>
      <div className="appcenter-rail">
        <div className="vlist__title" style={{padding:"4px 8px"}}>SOURCES</div>
        <button className={src==="all"?"is-on":""} onClick={()=>setSrc("all")}>All marketplaces</button>
        {MARKETPLACES.map(m => (
          <button key={m.id} className={src===m.id?"is-on":""} onClick={()=>setSrc(m.id)}>
            {m.name}
            {!m.locked && <span className="muted" style={{marginLeft:"auto",fontSize:9}}>•</span>}
          </button>
        ))}
        <div className="vlist__title" style={{padding:"12px 8px 4px"}}>CATEGORIES</div>
        <button className={cat==="all"?"is-on":""} onClick={()=>setCat("all")}>All</button>
        {DISPLAY_CATEGORIES.map(([id, label]) => (
          <button key={id} className={cat===id?"is-on":""} onClick={()=>setCat(id)}>{label}</button>
        ))}
      </div>
      <div style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <input className="input" placeholder="Search plugins…" style={{width:240}}/>
          <span className="muted" style={{fontSize:11,marginLeft:"auto"}}>{list.length} plugins</span>
        </TBar2>
        <div className="appcards">
          {list.map(p => {
            const mkt = MARKETPLACES.find(m => m.id === p.source);
            return (
              <div key={p.name} className="mkt-card">
                <div className="mkt-card__head">
                  <div className="mkt-card__icon">{p.name.split("-").slice(-1)[0].slice(0,2).toUpperCase()}</div>
                  <div style={{flex:1,minWidth:0}}>
                    <div className="mkt-card__name">{p.name}</div>
                    <div className="mkt-card__author">{p.author} · v{p.ver}</div>
                  </div>
                  <span className={`trust-badge trust-badge--${mkt?.locked?"official":"community"}`}>
                    <Icon name="shield" size={9}/>
                    {mkt?.locked?"locked":"verified"}
                  </span>
                </div>
                <div className="mkt-card__desc">{p.desc}</div>
                <div className="mkt-card__foot">
                  <span className="pill" style={{fontSize:9}}>{(DISPLAY_CATEGORIES.find(c=>c[0]===p.displayCategory)||[null,p.displayCategory])[1]}</span>
                  <span className="muted mono" style={{fontSize:10}}>{(p.size/1e6).toFixed(1)} MB</span>
                  <button className="btn btn--sm btn--primary" style={{marginLeft:"auto"}} onClick={()=>onInstall(p)}>
                    {p.installed ? "Installed" : "Install"}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function InstallConsentDialog({plugin, onClose}) {
  // Backend scope strings (internal/auth/rbac.go). Form: nova:<domain>:<verb>.
  // Production should fetch human-readable descriptions from
  // GET /api/v1/plugins/index/{name}/manifest?version=…
  // (the engine's permissions summary already includes scope + description).
  const permDescs = {
    "nova:storage:read":               "Read pools, vdevs, datasets, capacity",
    "nova:storage:write":              "Create/modify pools, vdevs, datasets",
    "nova:network:read":               "Read interfaces, routes, RDMA state",
    "nova:network:write":              "Configure interfaces, routes, RDMA",
    "nova:system:read":                "Read host info — hostname, version, uptime",
    "nova:system:write":               "Modify system configuration",
    "nova:system:admin":               "Reboot or shut down the host",
    "nova:audit:read":                 "Read the audit log",
    "nova:scheduler:read":             "Read scheduled jobs",
    "nova:scheduler:write":            "Create or modify scheduled jobs",
    "nova:notifications:read":         "Read notifications",
    "nova:notifications:write":        "Send notifications",
    "nova:notifications.events:read":  "Read notification event stream",
    "nova:notifications.events:write": "Emit notification events",
    "nova:encryption:read":            "Read encryption status of pools/datasets",
    "nova:encryption:write":           "Rotate encryption keys",
    "nova:encryption:recover":         "Unlock encrypted pools/datasets",
    "nova:replication:read":           "Read replication jobs and targets",
    "nova:replication:write":          "Create or modify replication jobs",
    "nova:scrub:read":                 "Read scrub status and policies",
    "nova:scrub:write":                "Trigger scrubs or change policies",
    "nova:alerts:read":                "Read fired alerts and silences",
    "nova:alerts:write":               "Acknowledge or silence alerts",
    "nova:logs:read":                  "Read system logs",
    "nova:sessions:read":              "Read active user sessions",
    "nova:sessions:admin":             "Revoke user sessions",
    "nova:vm:read":                    "Read VM list and state",
    "nova:vm:write":                   "Start, stop, or modify VMs",
    "nova:workloads:read":             "Read Helm workloads on the embedded cluster",
    "nova:workloads:write":            "Install or modify workloads",
    "nova:plugins:read":               "Read installed plugins",
    "nova:plugins:write":              "Install or remove plugins",
    "nova:plugins:admin":              "Modify plugin engine configuration",
    "nova:marketplaces:read":          "Read configured marketplaces",
    "nova:marketplaces:admin":         "Add or remove marketplaces",
  };
  const mkt = MARKETPLACES.find(m => m.id === plugin.source);
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={e=>e.stopPropagation()}>
        <div className="modal__head">
          <div className="mkt-card__icon" style={{width:36,height:36,fontSize:11}}>{plugin.name.split("-").slice(-1)[0].slice(0,2).toUpperCase()}</div>
          <div style={{flex:1}}>
            <div className="modal__title">Install {plugin.name}?</div>
            <div className="muted" style={{fontSize:11,marginTop:2}}>v{plugin.ver} · from <span style={{color:"var(--fg-1)"}}>{mkt?.name}</span></div>
          </div>
        </div>
        <div className="modal__body">
          <div style={{fontSize:11,color:"var(--fg-2)",marginBottom:14,lineHeight:1.5}}>{plugin.desc}</div>
          <Sect2 title={`Permissions (${plugin.perms.length})`}>
            <div style={{marginBottom:10}}>
              {plugin.perms.map(p => (
                <div key={p} className="perm-row">
                  <Icon name="shield" size={11}/>
                  <span className="perm-row__name">{p}</span>
                  <span className="perm-row__desc">{permDescs[p] || "—"}</span>
                </div>
              ))}
            </div>
          </Sect2>
          {plugin.deps && plugin.deps.length > 0 && (
            <Sect2 title="Dependencies">
              <div className="muted" style={{fontSize:11,marginBottom:6}}>Will also install:</div>
              {plugin.deps.map(d => (
                <div key={d} className="perm-row" style={{background:"var(--accent-soft)"}}>
                  <Icon name="apps" size={11}/>
                  <span className="perm-row__name">{d}</span>
                </div>
              ))}
            </Sect2>
          )}
          <Sect2 title="Trust">
            <div className="muted" style={{fontSize:11,lineHeight:1.5}}>
              Tarball verified by cosign against the marketplace
              <span className="mono" style={{color:"var(--fg-1)"}}> {mkt?.id}</span>
              <br/>Trust key: <span className="mono" style={{color:"var(--fg-1)",fontSize:10}}>{mkt?.trustKeyFingerprint}</span>
            </div>
          </Sect2>
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={onClose}><Icon name="download" size={10}/>Install · grant permissions</button>
        </div>
      </div>
    </div>
  );
}

function InstalledPlugins() {
  const [sel, setSel] = React.useState("novanas-photo-ai");
  const cur = PLUGINS.find(p => p.name === sel);
  return (
    <div style={{display:"grid",gridTemplateColumns:"1fr 320px",height:"100%"}}>
      <div style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <button className="btn">Check for upgrades</button>
          <span className="muted" style={{fontSize:11,marginLeft:"auto"}}>{PLUGINS.length} installed</span>
        </TBar2>
        <table className="tbl">
          <thead><tr><th>Plugin</th><th>Version</th><th>Source</th><th className="num">Permissions</th><th>Status</th></tr></thead>
          <tbody>{PLUGINS.map(p => (
            <tr key={p.name} className={sel===p.name?"is-on":""} onClick={()=>setSel(p.name)}>
              <td>{p.name}</td>
              <td className="mono">{p.ver}</td>
              <td className="muted">{p.source}</td>
              <td className="num mono">{p.perms.length}</td>
              <td><span className={`pill pill--${p.status==="running"?"ok":p.status==="error"?"err":""}`}><span className="dot"/>{p.status}</span></td>
            </tr>
          ))}</tbody>
        </table>
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div><div className="muted mono" style={{fontSize:10}}>PLUGIN</div><div className="side-detail__title">{cur.name}</div></div>
          </div>
          <Sect2 title="Status">
            <span className={`pill pill--${cur.status==="running"?"ok":"err"}`}><span className="dot"/>{cur.status}</span>
            <div className="muted" style={{fontSize:11,marginTop:6}}>v{cur.ver} from {cur.source}</div>
            <div className="muted" style={{fontSize:11}}>updated {cur.updated}</div>
          </Sect2>
          <Sect2 title="Permissions">
            {cur.perms.map(p => (
              <div key={p} className="perm-row"><Icon name="shield" size={10}/><span className="perm-row__name">{p}</span></div>
            ))}
          </Sect2>
          {cur.deps.length > 0 && (
            <Sect2 title="Depends on">
              {cur.deps.map(d => <div key={d} className="perm-row"><Icon name="apps" size={10}/><span className="perm-row__name">{d}</span></div>)}
            </Sect2>
          )}
          <div className="row gap-8" style={{padding:"10px 12px",borderTop:"1px solid var(--line)",flexWrap:"wrap"}}>
            <button className="btn btn--sm">Restart</button>
            <button className="btn btn--sm">Logs</button>
            <button className="btn btn--sm">Configure</button>
            <button className="btn btn--sm btn--danger" style={{marginLeft:"auto"}}>Uninstall</button>
          </div>
        </div>
      )}
    </div>
  );
}

function MarketplacesTab() {
  return (
    <div style={{padding:14}}>
      <TBar2>
        <button className="btn btn--primary"><Icon name="plus" size={11}/>Add marketplace</button>
        <button className="btn"><Icon name="refresh" size={11}/>Refresh trust keys</button>
      </TBar2>
      <table className="tbl">
        <thead><tr><th>Marketplace</th><th>URL</th><th>Trust fingerprint</th><th className="num">Plugins</th><th>Last sync</th><th>Status</th></tr></thead>
        <tbody>{MARKETPLACES.map(m => (
          <tr key={m.id}>
            <td>
              <div className="row gap-8">
                {m.locked && <Icon name="shield" size={11} style={{color:"var(--accent)"}}/>}
                {m.name}
                {m.locked && <span className="trust-badge trust-badge--official">locked</span>}
              </div>
            </td>
            <td className="mono muted" style={{fontSize:11}}>{m.url}</td>
            <td className="mono muted" style={{fontSize:10}}>{m.trustKeyFingerprint}</td>
            <td className="num mono">{m.pluginCount}</td>
            <td className="muted">{m.added}</td>
            <td><span className={`pill pill--${m.enabled?"ok":"warn"}`}><span className="dot"/>{m.enabled?"enabled":"disabled"}</span></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ ALERTS ═════════
function Alerts() {
  const [tab, setTab] = React.useState("active");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["active","silences","receivers"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="active" && <ActiveAlerts/>}
        {tab==="silences" && <Silences/>}
        {tab==="receivers" && <Receivers/>}
      </div>
    </div>
  );
}

function ActiveAlerts() {
  const [sel, setSel] = React.useState(ALERTS[0]?.fp);
  const cur = ALERTS.find(a => a.fp === sel);
  return (
    <div style={{display:"grid",gridTemplateColumns:"1fr 320px",height:"100%"}}>
      <div style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <span className="pill pill--err"><span className="dot"/>{ALERTS.filter(a=>a.severity==="critical").length} critical</span>
          <span className="pill pill--warn"><span className="dot"/>{ALERTS.filter(a=>a.severity==="warning").length} warning</span>
          <span className="pill pill--info"><span className="dot"/>{ALERTS.filter(a=>a.severity==="info").length} info</span>
          <button className="btn btn--sm" style={{marginLeft:"auto"}}><Icon name="refresh" size={11}/>Refresh</button>
        </TBar2>
        <table className="tbl">
          <thead><tr><th>Alert</th><th>Severity</th><th>Since</th><th>Labels</th></tr></thead>
          <tbody>{ALERTS.map(a => (
            <tr key={a.fp} className={sel===a.fp?"is-on":""} onClick={()=>setSel(a.fp)}>
              <td>{a.name}</td>
              <td><span className={`pill pill--${a.severity==="critical"?"err":a.severity==="warning"?"warn":"info"}`}><span className="dot"/>{a.severity}</span></td>
              <td className="muted">{a.since}</td>
              <td className="mono muted" style={{fontSize:10}}>{Object.entries(a.labels).map(([k,v])=>`${k}=${v}`).join(" ")}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div><div className="muted mono" style={{fontSize:10}}>ALERT · {cur.fp}</div><div className="side-detail__title">{cur.name}</div></div>
          </div>
          <Sect2 title="Summary"><div style={{fontSize:12}}>{cur.summary}</div></Sect2>
          <Sect2 title="Labels">
            <table className="tbl tbl--compact"><tbody>
              {Object.entries(cur.labels).map(([k,v]) => (<tr key={k}><td className="mono">{k}</td><td className="mono">{v}</td></tr>))}
            </tbody></table>
          </Sect2>
          <Sect2 title="State">
            <dl className="kv">
              <dt>Severity</dt><dd>{cur.severity}</dd>
              <dt>State</dt><dd>{cur.state}</dd>
              <dt>Since</dt><dd>{cur.since}</dd>
            </dl>
          </Sect2>
          <div className="row gap-8" style={{padding:"10px 12px",borderTop:"1px solid var(--line)"}}>
            <button className="btn btn--sm">Silence…</button>
            <button className="btn btn--sm">View runbook</button>
          </div>
        </div>
      )}
    </div>
  );
}

function Silences() {
  return (
    <div style={{padding:14}}>
      <TBar2><button className="btn btn--primary"><Icon name="plus" size={11}/>New silence</button></TBar2>
      <table className="tbl">
        <thead><tr><th>ID</th><th>Matchers</th><th>Comment</th><th>Creator</th><th>Ends</th><th></th></tr></thead>
        <tbody>{ALERT_SILENCES.map(s => (
          <tr key={s.id}>
            <td className="mono" style={{fontSize:11}}>{s.id}</td>
            <td className="mono" style={{fontSize:11}}>{s.matchers.map(m => `${m.n}=${m.v}`).join(" ")}</td>
            <td className="muted">{s.comment}</td>
            <td>{s.creator}</td>
            <td className="muted">{s.ends}</td>
            <td className="num"><button className="btn btn--sm btn--danger">Expire</button></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

function Receivers() {
  return (
    <div style={{padding:14}}>
      <TBar2><button className="btn btn--primary"><Icon name="plus" size={11}/>Add receiver</button></TBar2>
      <table className="tbl">
        <thead><tr><th>Receiver</th><th>Integrations</th></tr></thead>
        <tbody>{ALERT_RECEIVERS.map(r => (
          <tr key={r.name}>
            <td>{r.name}</td>
            <td><div className="row gap-4">{r.integrations.map(i=><span key={i} className="pill pill--info">{i}</span>)}</div></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ LOGS (Loki) ═════════
function Logs() {
  const [q, setQ] = React.useState('{job="systemd"} |~ "(?i)error"');
  const [running, setRunning] = React.useState(true);
  return (
    <div className="app-storage">
      <div style={{padding:10,borderBottom:"1px solid var(--line)",display:"flex",gap:8,alignItems:"center"}}>
        <span className="mono muted" style={{fontSize:11}}>LogQL</span>
        <input className="input" value={q} onChange={e=>setQ(e.target.value)} style={{flex:1,fontFamily:"var(--font-mono)"}}/>
        <button className="btn btn--sm">Last 1h</button>
        <button className={`btn btn--sm ${running?"btn--primary":""}`} onClick={()=>setRunning(r=>!r)}>
          <Icon name={running?"pause":"play"} size={9}/>{running?"Live tail":"Paused"}
        </button>
      </div>
      <div style={{display:"grid",gridTemplateColumns:"180px 1fr",flex:1,minHeight:0}}>
        <div style={{borderRight:"1px solid var(--line)",padding:8,overflow:"auto"}}>
          <div className="vlist__title">LABELS</div>
          {LOG_LABELS.map(l => (
            <div key={l} className="vlist__item">
              <Icon name="filter" size={10}/>{l}
              <span className="muted" style={{marginLeft:"auto",fontSize:9}}>{Math.floor(Math.random()*40)+5}</span>
            </div>
          ))}
        </div>
        <div className="log-stream">
          {LOG_LINES.map((l,i) => (
            <div key={i} className="log-line">
              <div className="log-line__t">{l.t}</div>
              <div className={`log-line__lvl log-line__lvl--${l.level}`}>{l.level}</div>
              <div className="log-line__unit">{l.unit}</div>
              <div className="log-line__msg">{l.msg}</div>
            </div>
          ))}
          {running && <div className="log-line"><div></div><div></div><div></div><div className="muted">▎ live tailing…</div></div>}
        </div>
      </div>
    </div>
  );
}

// ═════════ AUDIT ═════════
function Audit() {
  return (
    <div className="app-storage">
      <div className="win-body" style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <input className="input" placeholder="Search audit log…" style={{width:240}}/>
          <button className="btn btn--sm">Export CSV</button>
        </TBar2>
        <table className="tbl">
          <thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Resource</th><th>Result</th><th>IP</th></tr></thead>
          <tbody>{AUDIT.map((a,i) => (
            <tr key={i}>
              <td className="muted">{a.at}</td>
              <td>{a.actor}</td>
              <td className="mono" style={{fontSize:11}}>{a.action}</td>
              <td className="muted mono" style={{fontSize:11}}>{a.resource}</td>
              <td>{a.result==="ok"?<span className="pill pill--ok"><span className="dot"/>ok</span>:<span className="pill pill--err"><span className="dot"/>{a.result}</span>}</td>
              <td className="mono muted" style={{fontSize:11}}>{a.ip}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </div>
  );
}

// ═════════ JOBS ═════════
function JobsApp() {
  return (
    <div className="app-storage">
      <div className="win-body" style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <span className="pill pill--info"><span className="dot"/>{JOBS.filter(j=>j.state==="running").length} running</span>
          <span className="pill"><span className="dot"/>{JOBS.filter(j=>j.state==="queued").length} queued</span>
          <span className="pill pill--ok"><span className="dot"/>{JOBS.filter(j=>j.state==="ok").length} done</span>
        </TBar2>
        <table className="tbl">
          <thead><tr><th>Job</th><th>Kind</th><th>Target</th><th>Progress</th><th>ETA</th><th>State</th></tr></thead>
          <tbody>{JOBS.map(j => (
            <tr key={j.id}>
              <td className="mono" style={{fontSize:11}}>{j.id}</td>
              <td className="mono" style={{fontSize:11}}>{j.kind}</td>
              <td className="muted">{j.target}</td>
              <td>
                {j.state==="running" ? (
                  <div className="cap">
                    <div className="cap__bar"><div style={{width:`${j.pct*100}%`}}/></div>
                    <span className="mono" style={{fontSize:11}}>{Math.round(j.pct*100)}%</span>
                  </div>
                ) : <span className="muted">—</span>}
              </td>
              <td className="muted mono" style={{fontSize:11}}>{j.eta || "—"}</td>
              <td><span className={`pill pill--${j.state==="ok"?"ok":j.state==="running"?"info":j.state==="failed"?"err":""}`}><span className="dot"/>{j.state}</span></td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </div>
  );
}

// ═════════ NOTIFICATION CENTER ═════════
function NotificationCenter() {
  return (
    <div className="app-storage">
      <div className="win-body" style={{padding:14,overflow:"auto"}}>
        <TBar2>
          <span className="muted" style={{fontSize:11}}>{NOTIFICATIONS.filter(n=>!n.read).length} unread</span>
          <button className="btn btn--sm">Mark all read</button>
          <button className="btn btn--sm" style={{marginLeft:"auto"}}>Settings</button>
        </TBar2>
        <table className="tbl">
          <thead><tr><th></th><th>Time</th><th>Source</th><th>Message</th><th>Actor</th><th></th></tr></thead>
          <tbody>{NOTIFICATIONS.map(n => (
            <tr key={n.id} style={{opacity:n.read?0.55:1}}>
              <td><span className={`sdot sdot--${n.sev==="error"?"err":n.sev==="warn"?"warn":n.sev==="ok"?"ok":"info"}`}/></td>
              <td className="muted">{n.at}</td>
              <td><span className="pill" style={{fontSize:9}}>{n.src}</span></td>
              <td>{n.title}</td>
              <td className="muted">{n.actor}</td>
              <td className="num"><button className="btn btn--sm">Snooze</button></td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </div>
  );
}

// ═════════ SYSTEM ═════════
function SystemApp() {
  const [tab, setTab] = React.useState("overview");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["overview","updates","smtp","timezone"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:14,overflow:"auto"}}>
        {tab==="overview" && (
          <>
            <Sect2 title="Host">
              <dl className="kv">
                <dt>Hostname</dt><dd className="mono">{SYSTEM_INFO.hostname}</dd>
                <dt>Version</dt><dd>{SYSTEM_INFO.version}</dd>
                <dt>Kernel</dt><dd className="mono">{SYSTEM_INFO.kernel}</dd>
                <dt>OS</dt><dd>{SYSTEM_INFO.os}</dd>
                <dt>Uptime</dt><dd>{SYSTEM_INFO.uptime}</dd>
                <dt>Timezone</dt><dd className="mono">{SYSTEM_INFO.tz}</dd>
              </dl>
            </Sect2>
            <Sect2 title="Hardware">
              <dl className="kv">
                <dt>CPU</dt><dd>{SYSTEM_INFO.cpu}</dd>
                <dt>Cores / threads</dt><dd className="mono">{SYSTEM_INFO.cores} / {SYSTEM_INFO.threads}</dd>
                <dt>Memory</dt><dd className="mono">{SYSTEM_INFO.memory}</dd>
                <dt>BMC</dt><dd className="mono">{SYSTEM_INFO.bmc}</dd>
              </dl>
            </Sect2>
            <Sect2 title="Security">
              <dl className="kv">
                <dt>TPM</dt><dd>{SYSTEM_INFO.tpm}</dd>
                <dt>Secure Boot</dt><dd>{SYSTEM_INFO.secureBoot}</dd>
              </dl>
            </Sect2>
          </>
        )}
        {tab==="updates" && (
          <>
            <Sect2 title="Channel" action={<span className="pill pill--info">{SYSTEM_UPDATE.channel}</span>}>
              <dl className="kv">
                <dt>Current version</dt><dd className="mono">{SYSTEM_UPDATE.current}</dd>
                <dt>Available</dt><dd className="mono" style={{color:"var(--ok)"}}>{SYSTEM_UPDATE.next}</dd>
                <dt>Last check</dt><dd>{SYSTEM_UPDATE.checked}</dd>
              </dl>
            </Sect2>
            <Sect2 title={`Changelog · ${SYSTEM_UPDATE.next}`}>
              <ul style={{paddingLeft:18,fontSize:11,color:"var(--fg-2)",lineHeight:1.7,margin:0}}>
                {SYSTEM_UPDATE.notes.map((n,i) => <li key={i}>{n}</li>)}
              </ul>
            </Sect2>
            <div className="row gap-8" style={{marginTop:8}}>
              <button className="btn btn--primary"><Icon name="download" size={11}/>Install {SYSTEM_UPDATE.next}</button>
              <button className="btn">Check for updates</button>
            </div>
          </>
        )}
        {tab==="smtp" && (
          <>
            <Sect2 title="Outgoing relay" action={<span className={`pill pill--${SMTP_CONFIG.enabled?"ok":""}`}><span className="dot"/>{SMTP_CONFIG.enabled?"enabled":"disabled"}</span>}>
              <dl className="kv">
                <dt>Host</dt><dd className="mono">{SMTP_CONFIG.host}</dd>
                <dt>Port</dt><dd className="mono">{SMTP_CONFIG.port}</dd>
                <dt>Encryption</dt><dd>{SMTP_CONFIG.encryption}</dd>
                <dt>From</dt><dd className="mono">{SMTP_CONFIG.from}</dd>
                <dt>Auth</dt><dd className="mono">{SMTP_CONFIG.user}</dd>
                <dt>Last test</dt><dd>{SMTP_CONFIG.lastTest}</dd>
              </dl>
            </Sect2>
            <div className="row gap-8"><button className="btn btn--primary">Send test email</button><button className="btn">Edit</button></div>
          </>
        )}
        {tab==="timezone" && (
          <Sect2 title="Time">
            <dl className="kv">
              <dt>Timezone</dt><dd className="mono">{SYSTEM_INFO.tz}</dd>
              <dt>NTP</dt><dd>active · pool.ntp.org</dd>
              <dt>Drift</dt><dd className="mono">+1.2 ms</dd>
            </dl>
          </Sect2>
        )}
      </div>
    </div>
  );
}

// ═════════ FILE STATION (slim) ═════════
function FileStationApp() {
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

// ═════════ TERMINAL ═════════
function TerminalApp2() {
  return (
    <div className="app-term">
      <div className="term-line"><span className="term-prompt">novanas:~$</span> zpool status</div>
      <div className="term-out">  pool: fast<br/> state: ONLINE<br/>  scan: scrub repaired 0B in 2h14m</div>
      <div className="term-line"><span className="term-prompt">novanas:~$</span> kubectl get pods -n apps</div>
      <div className="term-out">NAME              READY   STATUS    RESTARTS   AGE<br/>immich-app-0      1/1     Running   0          2d<br/>immich-worker-0   1/1     Running   0          2d<br/>plex-1            1/1     Running   0          12d</div>
      <div className="term-line"><span className="term-prompt">novanas:~$</span> <span className="term-cursor">_</span></div>
    </div>
  );
}

window.WorkloadsApp = WorkloadsApp;
window.Virt2 = Virt2;
window.PackageCenter = PackageCenter;
window.Alerts = Alerts;
window.Logs = Logs;
window.Audit = Audit;
window.JobsApp = JobsApp;
window.NotificationCenter = NotificationCenter;
window.SystemApp = SystemApp;
window.FileStationApp = FileStationApp;
window.TerminalApp2 = TerminalApp2;
