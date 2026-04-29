/* globals React, Icon, Spark, useSeries, Ring,
   POOLS, VDEV_TREE, DISKS, DATASETS, SNAPSHOTS, SNAPSHOT_SCHEDULES, SCRUB_POLICIES,
   REPL_TARGETS, REPL_JOBS, ENCRYPTED_DATASETS,
   NETWORK_INTERFACES, RDMA_DEVICES,
   APPS, WORKLOADS, VMS, VM_TEMPLATES, VM_SNAPSHOTS, PLUGINS, MARKETPLACE_PLUGINS, MARKETPLACES,
   USERS, SESSIONS, LOGIN_HISTORY, KRB5_PRINCIPALS,
   ALERTS, ALERT_SILENCES, ALERT_RECEIVERS, LOG_LABELS, LOG_LINES, AUDIT, JOBS, NOTIFICATIONS,
   NFS_EXPORTS, SMB_SHARES, ISCSI_TARGETS, NVMEOF_SUBSYSTEMS, PROTOCOL_SHARES,
   SYSTEM_INFO, SYSTEM_UPDATE, SMTP_CONFIG, ACTIVITY, FILES,
   fmtBytes, fmtPct */

const Sect = ({title, action, children}) => (
  <div className="sect">
    <div className="sect__head"><div className="sect__title">{title}</div>{action}</div>
    <div className="sect__body">{children}</div>
  </div>
);
const TBar = ({children}) => <div className="tbar">{children}</div>;
const Empty = ({children}) => <div className="empty-hint">{children}</div>;

// ═════════ STORAGE / POOLS ═════════
function StorageManager() {
  const [tab, setTab] = React.useState("pools");
  const [poolSel, setPoolSel] = React.useState("fast");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["pools","vdev","disks","datasets","snapshots","encryption"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={() => setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab === "pools" && <PoolsTab onPick={(n)=>{setPoolSel(n); setTab("vdev");}}/>}
        {tab === "vdev" && <VdevTab pool={poolSel} setPool={setPoolSel}/>}
        {tab === "disks" && <DisksTab/>}
        {tab === "datasets" && <DatasetsTab/>}
        {tab === "snapshots" && <SnapshotsTab/>}
        {tab === "encryption" && <EncryptionTab/>}
      </div>
    </div>
  );
}

function PoolsTab({onPick}) {
  return (
    <div style={{padding:14}}>
      <TBar>
        <button className="btn btn--primary"><Icon name="plus" size={11}/>Create pool</button>
        <button className="btn"><Icon name="download" size={11}/>Import</button>
        <span className="muted" style={{marginLeft:"auto",fontSize:11}}>{POOLS.length} pools · {fmtBytes(POOLS.reduce((m,p)=>m+p.total,0))} total</span>
      </TBar>
      <div className="cards-grid">
        {POOLS.map(p => {
          const pct = p.used / p.total;
          return (
            <div key={p.name} className="pool-card" onClick={()=>onPick(p.name)} style={{cursor:"pointer"}}>
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
              <div className="pool-card__scrub">
                <span className="muted">scrub: {p.scrubLast}</span>
                <span className="muted">next: {p.scrubNext}</span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function VdevTab({pool, setPool}) {
  const tree = VDEV_TREE[pool] || [];
  const cur = POOLS.find(p => p.name === pool);
  return (
    <div style={{padding:14, display:"grid", gridTemplateColumns:"160px 1fr", gap:14}}>
      <div className="vlist">
        <div className="vlist__title">POOLS</div>
        {POOLS.map(p => (
          <button key={p.name} className={`vlist__item ${pool===p.name?"is-on":""}`} onClick={()=>setPool(p.name)}>
            <span className={`tier-mark tier-mark--${p.tier}`}/>{p.name}
          </button>
        ))}
      </div>
      <div className="col gap-12">
        <div className="row gap-8" style={{flexWrap:"wrap"}}>
          <span className="pill pill--ok"><span className="dot"/>{cur.state}</span>
          <span className="pill">{cur.protection}</span>
          <span className="pill">{cur.devices}</span>
          <button className="btn btn--sm" style={{marginLeft:"auto"}}><Icon name="play" size={9}/>Scrub now</button>
          <button className="btn btn--sm"><Icon name="bolt" size={9}/>Trim</button>
          <button className="btn btn--sm"><Icon name="more" size={11}/></button>
        </div>
        <Sect title="VDEV layout">
          <table className="tbl tbl--compact">
            <thead><tr><th>VDEV</th><th>Type</th><th>State</th><th>Disks</th></tr></thead>
            <tbody>
              {tree.map(v => (
                <tr key={v.name}>
                  <td className="mono">{v.name}</td>
                  <td><span className={`pill pill--${v.type.startsWith("mirror")?"info":v.type.startsWith("raidz")?"warn":""}`}>{v.type}</span></td>
                  <td><span className={`sdot sdot--${v.state==="ONLINE"||v.state==="AVAIL"?"ok":"warn"}`}/> {v.state}</td>
                  <td className="mono" style={{fontSize:11}}>{v.disks.join(" · ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Sect>
        <Sect title="I/O">
          <div className="row gap-12" style={{flexWrap:"wrap"}}>
            <div className="kpi"><div className="kpi__lbl">Read</div><div className="kpi__val mono">{cur.throughput.r} <span className="muted">MB/s</span></div></div>
            <div className="kpi"><div className="kpi__lbl">Write</div><div className="kpi__val mono">{cur.throughput.w} <span className="muted">MB/s</span></div></div>
            <div className="kpi"><div className="kpi__lbl">Read IOPS</div><div className="kpi__val mono">{(cur.iops.r/1000).toFixed(1)}k</div></div>
            <div className="kpi"><div className="kpi__lbl">Write IOPS</div><div className="kpi__val mono">{(cur.iops.w/1000).toFixed(1)}k</div></div>
          </div>
        </Sect>
      </div>
    </div>
  );
}

function DisksTab() {
  const [sel, setSel] = React.useState(13);
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
        {selected.state === "EMPTY" ? (
          <Empty>Empty slot · insert a disk to initialize</Empty>
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
              <dt>Serial</dt><dd>{selected.serial}</dd>
              <dt>Capacity</dt><dd>{fmtBytes(selected.cap)}</dd>
              <dt>Pool</dt><dd>{selected.pool}</dd>
              <dt>Temperature</dt><dd>{selected.temp}°C</dd>
              <dt>Power-on hours</dt><dd>{selected.hours.toLocaleString()}</dd>
              <dt>SMART pass</dt><dd>{selected.smart.passed ? "yes" : "no"}</dd>
              <dt>Reallocated</dt><dd style={{color: selected.smart.reallocated>0?"var(--warn)":undefined}}>{selected.smart.reallocated}</dd>
              <dt>Pending</dt><dd>{selected.smart.pending}</dd>
              {selected.reason && <><dt style={{color:"var(--warn)"}}>Notice</dt><dd style={{color:"var(--warn)"}}>{selected.reason}</dd></>}
            </dl>
            <div className="row gap-8" style={{flexWrap:"wrap"}}>
              <button className="btn btn--sm">Run SMART · short</button>
              <button className="btn btn--sm">Run SMART · long</button>
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
  const [sel, setSel] = React.useState(null);
  return (
    <div style={{display:"grid", gridTemplateColumns: sel?"1fr 320px":"1fr", height:"100%"}}>
      <div style={{overflow:"auto"}}>
        <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New dataset</button></TBar>
        <table className="tbl">
          <thead><tr>
            <th>Dataset</th><th>Pool</th><th>Protocol</th><th className="num">Used</th><th>Quota</th><th className="num">Snaps</th><th>Comp</th><th>Enc</th>
          </tr></thead>
          <tbody>
            {DATASETS.map(d => {
              const pct = d.used/d.quota;
              return (
                <tr key={d.name} onClick={()=>setSel(d.name)} className={sel===d.name?"is-on":""}>
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
                  <td className="muted mono" style={{fontSize:11}}>{d.comp}</td>
                  <td>{d.enc ? <Icon name="shield" size={12}/> : <span className="muted">—</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {sel && <DatasetDetail name={sel} onClose={()=>setSel(null)}/>}
    </div>
  );
}

function DatasetDetail({name, onClose}) {
  const d = DATASETS.find(x => x.name === name);
  if (!d) return null;
  const pct = d.used/d.quota;
  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div><div className="muted mono" style={{fontSize:10}}>DATASET</div><div className="side-detail__title">{d.name}</div></div>
        <button className="btn btn--sm" onClick={onClose}><Icon name="close" size={10}/></button>
      </div>
      <Sect title="Capacity">
        <div className="bar"><div style={{width:`${pct*100}%`}}/></div>
        <div className="row" style={{justifyContent:"space-between",fontSize:11,marginTop:4}}>
          <span className="mono">{fmtBytes(d.used)}</span>
          <span className="muted mono">/ {fmtBytes(d.quota)}</span>
        </div>
      </Sect>
      <Sect title="Properties">
        <dl className="kv">
          <dt>Pool</dt><dd>{d.pool}</dd>
          <dt>Protocol</dt><dd>{d.proto}</dd>
          <dt>Compression</dt><dd>{d.comp}</dd>
          <dt>Recordsize</dt><dd>{d.recordsize}</dd>
          <dt>Atime</dt><dd>{d.atime}</dd>
          <dt>Encrypted</dt><dd>{d.enc?"yes (TPM-sealed)":"no"}</dd>
          <dt>Snapshots</dt><dd>{d.snap}</dd>
        </dl>
      </Sect>
      <Sect title="ACL · NFSv4">
        <table className="tbl tbl--compact">
          <tbody>
            <tr><td className="mono">@family</td><td>rwx</td><td className="muted">allow</td></tr>
            <tr><td className="mono">@guests</td><td>r-x</td><td className="muted">allow</td></tr>
            <tr><td className="mono">pascal</td><td>rwx</td><td className="muted">allow</td></tr>
          </tbody>
        </table>
      </Sect>
      <div className="row gap-8" style={{padding:"10px 12px",borderTop:"1px solid var(--line)"}}>
        <button className="btn btn--sm">Snapshot</button>
        <button className="btn btn--sm">Send…</button>
        <button className="btn btn--sm btn--danger" style={{marginLeft:"auto"}}>Destroy</button>
      </div>
    </div>
  );
}

function SnapshotsTab() {
  return (
    <div style={{padding:14}}>
      <TBar>
        <button className="btn btn--primary"><Icon name="plus" size={11}/>New snapshot</button>
        <button className="btn"><Icon name="refresh" size={11}/>Send/Receive</button>
        <input className="input" placeholder="Filter snapshots…" style={{marginLeft:"auto",width:180}}/>
      </TBar>
      <table className="tbl">
        <thead><tr><th>Snapshot</th><th>Pool</th><th className="num">Size</th><th>Schedule</th><th>Hold</th><th>Created</th><th></th></tr></thead>
        <tbody>{SNAPSHOTS.map(s => (
          <tr key={s.name}>
            <td className="mono" style={{fontSize:11}}>{s.name}</td>
            <td className="muted mono">{s.pool}</td>
            <td className="num mono">{fmtBytes(s.size)}</td>
            <td className="muted">{s.schedule}</td>
            <td>{s.hold ? <Icon name="shield" size={11}/> : <span className="muted">—</span>}</td>
            <td className="muted">{s.t}</td>
            <td className="num"><button className="btn btn--sm">Rollback</button></td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

function EncryptionTab() {
  return (
    <div style={{padding:14}}>
      <Sect title="TPM-sealed key escrow" action={<span className="pill pill--ok"><span className="dot"/>TPM 2.0 healthy</span>}>
        <div className="muted" style={{fontSize:11,marginBottom:10}}>Native ZFS encryption · keys are wrapped to PCRs of this host. Recovery requires admin role and is audit-logged.</div>
      </Sect>
      <table className="tbl">
        <thead><tr><th>Dataset</th><th>Status</th><th>Format</th><th>Key location</th><th>Last rotated</th><th></th></tr></thead>
        <tbody>{ENCRYPTED_DATASETS.map(e => (
          <tr key={e.name}>
            <td>{e.name}</td>
            <td><span className={`pill pill--${e.status==="available"?"ok":"warn"}`}><span className="dot"/>{e.status}</span></td>
            <td className="mono" style={{fontSize:11}}>{e.keyformat}</td>
            <td className="mono" style={{fontSize:11}}>{e.keylocation}</td>
            <td className="muted">{e.rotated}</td>
            <td className="num">
              <button className="btn btn--sm">{e.status==="available"?"Unload":"Load"}</button>
              <button className="btn btn--sm btn--danger" style={{marginLeft:4}}>Recover</button>
            </td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ REPLICATION ═════════
function Replication() {
  const [tab, setTab] = React.useState("jobs");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["jobs","targets","schedules"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="jobs" && <ReplJobs/>}
        {tab==="targets" && <ReplTargets/>}
        {tab==="schedules" && <SnapSchedules/>}
      </div>
    </div>
  );
}

function ReplJobs() {
  const [sel, setSel] = React.useState("r5");
  const cur = REPL_JOBS.find(j => j.id === sel);
  return (
    <div style={{display:"grid",gridTemplateColumns:"1fr 320px",height:"100%"}}>
      <div style={{padding:14,overflow:"auto"}}>
        <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New job</button></TBar>
        <table className="tbl">
          <thead><tr><th>Job</th><th>Source</th><th>Target</th><th>Schedule</th><th>State</th><th>Last run</th><th className="num">Bytes</th></tr></thead>
          <tbody>{REPL_JOBS.map(j => (
            <tr key={j.id} className={sel===j.id?"is-on":""} onClick={()=>setSel(j.id)}>
              <td>{j.name}</td>
              <td className="mono muted" style={{fontSize:11}}>{j.source}</td>
              <td className="mono muted" style={{fontSize:11}}>{j.target}</td>
              <td className="mono muted" style={{fontSize:11}}>{j.schedule}</td>
              <td><span className={`pill pill--${j.state==="OK"?"ok":j.state==="RUNNING"?"info":j.state==="FAILED"?"err":""}`}><span className="dot"/>{j.state}</span></td>
              <td className="muted">{j.lastRun}</td>
              <td className="num mono">{j.lastBytes ? fmtBytes(j.lastBytes) : "—"}</td>
            </tr>
          ))}</tbody>
        </table>
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div><div className="muted mono" style={{fontSize:10}}>JOB · {cur.id}</div><div className="side-detail__title">{cur.name}</div></div>
          </div>
          <Sect title="Status">
            <span className={`pill pill--${cur.state==="OK"?"ok":cur.state==="RUNNING"?"info":cur.state==="FAILED"?"err":""}`}><span className="dot"/>{cur.state}</span>
            {cur.error && <div className="muted" style={{color:"var(--err)",fontSize:11,marginTop:8}}>{cur.error}</div>}
          </Sect>
          <Sect title="Configuration">
            <dl className="kv">
              <dt>Source</dt><dd>{cur.source}</dd>
              <dt>Target</dt><dd>{cur.target}</dd>
              <dt>Schedule</dt><dd>{cur.schedule}</dd>
              <dt>Last bytes</dt><dd>{fmtBytes(cur.lastBytes)}</dd>
              <dt>Last duration</dt><dd>{cur.lastDur}</dd>
            </dl>
          </Sect>
          <div className="row gap-8" style={{padding:"10px 12px",borderTop:"1px solid var(--line)"}}>
            <button className="btn btn--sm btn--primary">Run now</button>
            <button className="btn btn--sm">Edit</button>
            <button className="btn btn--sm btn--danger" style={{marginLeft:"auto"}}>Delete</button>
          </div>
        </div>
      )}
    </div>
  );
}

function ReplTargets() {
  return (
    <div style={{padding:14}}>
      <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>Add target</button></TBar>
      <table className="tbl">
        <thead><tr><th>Name</th><th>Protocol</th><th>Host</th><th>Details</th></tr></thead>
        <tbody>{REPL_TARGETS.map(t => (
          <tr key={t.id}>
            <td>{t.name}</td>
            <td><span className="pill pill--info">{t.protocol}</span></td>
            <td className="mono">{t.host}</td>
            <td className="muted mono" style={{fontSize:11}}>{t.protocol==="s3"?`region=${t.region}`:`user=${t.ssh_user}, port=${t.port}`}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

function SnapSchedules() {
  return (
    <div style={{padding:14}}>
      <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New schedule</button></TBar>
      <table className="tbl">
        <thead><tr><th>Name</th><th>Datasets</th><th>Cron</th><th className="num">Keep</th><th>Enabled</th></tr></thead>
        <tbody>{SNAPSHOT_SCHEDULES.map(s => (
          <tr key={s.id}>
            <td>{s.name}</td>
            <td className="muted mono" style={{fontSize:11}}>{s.datasets.join(", ")}</td>
            <td className="mono">{s.cron}</td>
            <td className="num mono">{s.keep}</td>
            <td>{s.enabled ? <Icon name="check" size={11}/> : <span className="muted">off</span>}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

function ScrubPolicies() {
  return (
    <div style={{padding:14}}>
      <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New policy</button></TBar>
      <table className="tbl">
        <thead><tr><th>Name</th><th>Pools</th><th>Cron</th><th>Priority</th><th>Type</th></tr></thead>
        <tbody>{SCRUB_POLICIES.map(p => (
          <tr key={p.id}>
            <td>{p.name}</td>
            <td className="mono muted" style={{fontSize:11}}>{p.pools.join(", ")}</td>
            <td className="mono">{p.cron}</td>
            <td><span className={`pill pill--${p.priority==="high"?"warn":p.priority==="low"?"":"info"}`}>{p.priority}</span></td>
            <td className="muted">{p.builtin?"built-in":"custom"}</td>
          </tr>
        ))}</tbody>
      </table>
    </div>
  );
}

// ═════════ NETWORK ═════════
function NetworkApp() {
  const [tab, setTab] = React.useState("interfaces");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["interfaces","rdma","routes"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="interfaces" && (
          <div style={{padding:14}}>
            <TBar>
              <button className="btn btn--primary"><Icon name="plus" size={11}/>Add interface</button>
              <button className="btn">Add VLAN</button>
              <button className="btn">Add bond</button>
              <button className="btn" style={{marginLeft:"auto"}}><Icon name="refresh" size={11}/>Reload</button>
            </TBar>
            <table className="tbl">
              <thead><tr><th>Interface</th><th>Type</th><th>State</th><th>IPv4</th><th>MAC</th><th className="num">MTU</th><th>Speed</th></tr></thead>
              <tbody>{NETWORK_INTERFACES.map(i => (
                <tr key={i.name}>
                  <td className="mono">{i.name}</td>
                  <td><span className="pill pill--info">{i.type}</span></td>
                  <td><span className={`sdot sdot--${i.state==="UP"?"ok":"warn"}`}/> {i.state}</td>
                  <td className="mono" style={{fontSize:11}}>{i.ipv4 || <span className="muted">—</span>}</td>
                  <td className="mono muted" style={{fontSize:11}}>{i.mac}</td>
                  <td className="num mono">{i.mtu}</td>
                  <td className="mono" style={{fontSize:11}}>{i.speed}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="rdma" && (
          <div style={{padding:14}}>
            <table className="tbl">
              <thead><tr><th>Device</th><th>Port</th><th>State</th><th>Speed</th><th>GID</th></tr></thead>
              <tbody>{RDMA_DEVICES.map((r,i) => (
                <tr key={i}>
                  <td className="mono">{r.name}</td>
                  <td className="mono">{r.port}</td>
                  <td><span className={`sdot sdot--${r.state==="ACTIVE"?"ok":"warn"}`}/> {r.state}</td>
                  <td className="mono" style={{fontSize:11}}>{r.speed}</td>
                  <td className="mono muted" style={{fontSize:11}}>{r.gid}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="routes" && (
          <div style={{padding:14}}>
            <table className="tbl">
              <thead><tr><th>Destination</th><th>Gateway</th><th>Interface</th><th className="num">Metric</th></tr></thead>
              <tbody>
                <tr><td className="mono">default</td><td className="mono">192.168.1.1</td><td className="mono">bond0</td><td className="num mono">100</td></tr>
                <tr><td className="mono">192.168.1.0/24</td><td className="muted">link</td><td className="mono">bond0</td><td className="num mono">100</td></tr>
                <tr><td className="mono">10.0.10.0/24</td><td className="muted">link</td><td className="mono">ens3f0</td><td className="num mono">100</td></tr>
                <tr><td className="mono">10.20.0.0/24</td><td className="muted">link</td><td className="mono">vlan20</td><td className="num mono">100</td></tr>
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// ═════════ SHARES (SMB / NFS / iSCSI / NVMe-oF / unified) ═════════
function Shares() {
  const [tab, setTab] = React.useState("unified");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["unified","smb","nfs","iscsi","nvmeof"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="unified" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New share</button></TBar>
            <table className="tbl">
              <thead><tr><th>Share</th><th>Protocols</th><th>Path</th><th>Clients</th><th>State</th></tr></thead>
              <tbody>{PROTOCOL_SHARES.map(s => (
                <tr key={s.name}>
                  <td>{s.name}</td>
                  <td><div className="row gap-4">{s.protocols.map(p => <span key={p} className="pill pill--info">{p}</span>)}</div></td>
                  <td className="mono muted" style={{fontSize:11}}>{s.path}</td>
                  <td className="mono muted" style={{fontSize:11}}>{s.clients}</td>
                  <td><span className="pill pill--ok"><span className="dot"/>{s.state}</span></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="smb" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New SMB share</button><button className="btn">Globals…</button><button className="btn">Users…</button></TBar>
            <table className="tbl">
              <thead><tr><th>Share</th><th>Path</th><th>Users</th><th>Guest</th><th>Recycle</th><th>VFS</th></tr></thead>
              <tbody>{SMB_SHARES.map(s => (
                <tr key={s.name}>
                  <td>{s.name}</td>
                  <td className="mono muted" style={{fontSize:11}}>{s.path}</td>
                  <td className="mono">{s.users}</td>
                  <td>{s.guest ? <Icon name="check" size={11}/> : <span className="muted">no</span>}</td>
                  <td>{s.recycle ? <Icon name="check" size={11}/> : <span className="muted">no</span>}</td>
                  <td className="mono muted" style={{fontSize:11}}>{s.vfs || "—"}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="nfs" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New export</button><button className="btn">Reload</button></TBar>
            <table className="tbl">
              <thead><tr><th>Export</th><th>Path</th><th>Clients</th><th>Options</th><th>Active</th></tr></thead>
              <tbody>{NFS_EXPORTS.map(n => (
                <tr key={n.name}>
                  <td>{n.name}</td>
                  <td className="mono muted" style={{fontSize:11}}>{n.path}</td>
                  <td className="mono">{n.clients}</td>
                  <td className="mono muted" style={{fontSize:11}}>{n.options}</td>
                  <td>{n.active ? <span className="pill pill--ok"><span className="dot"/>up</span> : <span className="muted">off</span>}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="iscsi" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New target</button></TBar>
            <table className="tbl">
              <thead><tr><th>IQN</th><th className="num">LUNs</th><th>Portals</th><th className="num">ACLs</th><th>State</th></tr></thead>
              <tbody>{ISCSI_TARGETS.map(t => (
                <tr key={t.iqn}>
                  <td className="mono" style={{fontSize:11}}>{t.iqn}</td>
                  <td className="num mono">{t.luns}</td>
                  <td className="mono muted" style={{fontSize:11}}>{t.portals.join(", ")}</td>
                  <td className="num mono">{t.acls}</td>
                  <td><span className="pill pill--ok"><span className="dot"/>{t.state}</span></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="nvmeof" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New subsystem</button></TBar>
            <table className="tbl">
              <thead><tr><th>NQN</th><th className="num">NS</th><th className="num">Ports</th><th className="num">Hosts</th><th>DH-CHAP</th><th>State</th></tr></thead>
              <tbody>{NVMEOF_SUBSYSTEMS.map(s => (
                <tr key={s.nqn}>
                  <td className="mono" style={{fontSize:11}}>{s.nqn}</td>
                  <td className="num mono">{s.ns}</td>
                  <td className="num mono">{s.ports}</td>
                  <td className="num mono">{s.hosts}</td>
                  <td>{s.dhchap ? <Icon name="shield" size={11}/> : <span className="muted">off</span>}</td>
                  <td><span className="pill pill--ok"><span className="dot"/>{s.state}</span></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// ═════════ IDENTITY (Users / Sessions / Login / Kerberos) ═════════
function Identity() {
  const [tab, setTab] = React.useState("users");
  return (
    <div className="app-storage">
      <div className="win-tabs">
        {["users","sessions","login-history","kerberos"].map(t => (
          <button key={t} className={tab===t?"is-on":""} onClick={()=>setTab(t)}>{t}</button>
        ))}
      </div>
      <div className="win-body" style={{padding:0,overflow:"auto"}}>
        {tab==="users" && (
          <div style={{padding:14}}>
            <TBar><button className="btn btn--primary"><Icon name="plus" size={11}/>New user</button></TBar>
            <table className="tbl">
              <thead><tr><th>Username</th><th>Role</th><th>Email</th><th>MFA</th><th>Last login</th><th>Status</th></tr></thead>
              <tbody>{USERS.map(u => (
                <tr key={u.name}>
                  <td><div className="row gap-8"><div className="avatar">{u.name.slice(0,2).toUpperCase()}</div>{u.name}</div></td>
                  <td><span className={`pill pill--${u.role==="nova-admin"?"warn":u.role==="service"?"":"info"}`}>{u.role}</span></td>
                  <td className="muted">{u.email}</td>
                  <td>{u.mfa ? <Icon name="shield" size={11}/> : <span className="muted">no</span>}</td>
                  <td className="muted">{u.lastLogin}</td>
                  <td>{u.status==="active" ? <span className="pill pill--ok"><span className="dot"/>active</span> : <span className="pill"><span className="dot"/>disabled</span>}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="sessions" && (
          <div style={{padding:14}}>
            <table className="tbl">
              <thead><tr><th>Session</th><th>User</th><th>IP</th><th>Client</th><th>Started</th><th></th></tr></thead>
              <tbody>{SESSIONS.map(s => (
                <tr key={s.id}>
                  <td className="mono" style={{fontSize:11}}>{s.id}{s.current && <span className="pill pill--ok" style={{marginLeft:6}}>current</span>}</td>
                  <td>{s.user}</td>
                  <td className="mono">{s.ip}</td>
                  <td className="muted">{s.ua}</td>
                  <td className="muted">{s.started}</td>
                  <td className="num"><button className="btn btn--sm btn--danger" disabled={s.current}>Revoke</button></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="login-history" && (
          <div style={{padding:14}}>
            <table className="tbl">
              <thead><tr><th>When</th><th>User</th><th>IP</th><th>Method</th><th>Result</th></tr></thead>
              <tbody>{LOGIN_HISTORY.map((h,i) => (
                <tr key={i}>
                  <td className="muted">{h.at}</td>
                  <td>{h.user}</td>
                  <td className="mono">{h.ip}</td>
                  <td className="muted mono" style={{fontSize:11}}>{h.method}</td>
                  <td>{h.result==="success" ? <span className="pill pill--ok"><span className="dot"/>ok</span> : <span className="pill pill--err"><span className="dot"/>fail</span>}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
        {tab==="kerberos" && (
          <div style={{padding:14}}>
            <Sect title="Realm" action={<span className="pill pill--ok"><span className="dot"/>KDC online</span>}>
              <dl className="kv">
                <dt>Realm</dt><dd>LAN.NOVANAS.IO</dd>
                <dt>KDC</dt><dd>nas.lan:88</dd>
                <dt>Admin server</dt><dd>nas.lan:749</dd>
                <dt>Idmap</dt><dd>cfg loaded</dd>
              </dl>
            </Sect>
            <Sect title="Principals" action={<button className="btn btn--sm btn--primary"><Icon name="plus" size={9}/>New</button>}>
              <table className="tbl tbl--compact">
                <thead><tr><th>Principal</th><th>Type</th><th className="num">KVNO</th><th>Created</th><th>Expires</th><th></th></tr></thead>
                <tbody>{KRB5_PRINCIPALS.map(p => (
                  <tr key={p.name}>
                    <td className="mono" style={{fontSize:11}}>{p.name}</td>
                    <td><span className="pill">{p.type}</span></td>
                    <td className="num mono">{p.keyver}</td>
                    <td className="muted">{p.created}</td>
                    <td className="muted">{p.expires}</td>
                    <td className="num"><button className="btn btn--sm">Keytab</button></td>
                  </tr>
                ))}</tbody>
              </table>
            </Sect>
          </div>
        )}
      </div>
    </div>
  );
}

window.StorageManager = StorageManager;
window.Replication = Replication;
window.ScrubPolicies = ScrubPolicies;
window.NetworkApp = NetworkApp;
window.Shares = Shares;
window.Identity = Identity;
