/* globals React, Icon, Pill, StatusDot, Sparkline, RingMeter, CapacityBar, useTicker, useSparkSeries, fmtBytes, fmtNum, fmtPct, SEEDS, POOLS, DATASETS, ACTIVITY */

function DashboardScreen({ seed, role }) {
  const seedData = SEEDS[seed] || SEEDS.healthy;
  const tick = useTicker(2000);

  const readS  = useSparkSeries(48, 0.55, 0.18, 11, 1500);
  const writeS = useSparkSeries(48, 0.30, 0.15, 12, 1500);
  const iopsS  = useSparkSeries(48, 0.48, 0.22, 13, 1500);
  const netS   = useSparkSeries(48, 0.40, 0.20, 14, 1500);

  const readNow  = (readS[readS.length - 1]  * 2400).toFixed(0);
  const writeNow = (writeS[writeS.length - 1] * 1200).toFixed(0);
  const iopsNow  = Math.round(iopsS[iopsS.length - 1] * 52000);
  const netNow   = (netS[netS.length - 1] * 8.5).toFixed(1);

  const rebuildBanner = seedData.rebuild;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{role === "user" ? "My Dashboard" : "Dashboard"}</h1>
          <div className="page-head__sub">nas-01 · NovaNas 26.07.3 · Europe/Brussels</div>
        </div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="refresh" size={13}/> Refresh</button>
          <button className="btn"><Icon name="activity" size={13}/> Open Grafana</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> Create</button>
        </div>
      </div>

      {rebuildBanner && (
        <div className="banner banner--warn mb-12">
          <Icon name="shield" size={16}/>
          <div className="banner__text">
            <div className="banner__title">Pool <span className="mono">fast</span> is rebuilding onto spare in slot 8.</div>
            <div style={{ color: "var(--fg-2)", marginTop: 2 }}>
              Protection intact. ETA {rebuildBanner.eta} · progress {Math.round(rebuildBanner.progress*100)}%.
            </div>
          </div>
          <div style={{ minWidth: 220 }}>
            <div className="pbar"><div style={{ width: `${rebuildBanner.progress*100}%` }}/></div>
          </div>
          <button className="btn btn--sm">Details</button>
        </div>
      )}

      <div className="health-hero mb-12">
        <div className="health-hero__left">
          <div className={`health-hero__status`} style={{ color: seedData.status === "ok" ? "var(--ok)" : "var(--warn)" }}>
            <span className={`sdot sdot--${seedData.status}`} style={{ marginRight: 8 }}/>
            {seedData.status === "ok" ? "Operational" : "Attention required"}
          </div>
          <div className="health-hero__title">{seedData.statusLabel}</div>
          <div className="health-hero__sub">
            22 disks active · 4 pools online · Keycloak, OpenBao, k3s control plane nominal.
            Last scrub: <span className="mono fg0">3h ago</span>. Last config backup: <span className="mono fg0">14:00</span>.
          </div>
          <div className="health-hero__stats">
            <div>
              <div className="health-hero__stat__label">Capacity</div>
              <div className="health-hero__stat__value">42.1<span style={{ color: "var(--fg-3)", fontSize: 13 }}> / 98.1 TB</span></div>
              <div className="pbar mt-8" style={{ maxWidth: 160 }}><div style={{ width: "43%" }}/></div>
            </div>
            <div>
              <div className="health-hero__stat__label">Apps running</div>
              <div className="health-hero__stat__value">8<span style={{ color: "var(--fg-3)", fontSize: 13 }}> / 12 installed</span></div>
            </div>
            <div>
              <div className="health-hero__stat__label">VMs running</div>
              <div className="health-hero__stat__value">5<span style={{ color: "var(--fg-3)", fontSize: 13 }}> / 6 defined</span></div>
            </div>
          </div>
        </div>
        <div className="health-ring">
          <RingMeter value={0.429} label="43%" sub="of 98.1 TB usable"/>
        </div>
      </div>

      <div className="grid grid-4 mb-12">
        <MetricCard label="Read"  value={readNow}  unit="MB/s"  delta="+4.2%" up data={readS}  color="var(--accent)"/>
        <MetricCard label="Write" value={writeNow} unit="MB/s"  delta="-1.1%" data={writeS} color="oklch(0.72 0.14 320)"/>
        <MetricCard label="IOPS"  value={fmtNum(iopsNow, 1)} unit="" delta="+7.8%" up data={iopsS} color="oklch(0.78 0.14 160)"/>
        <MetricCard label="Network" value={netNow} unit="Gb/s" delta="+0.4%" up data={netS} color="oklch(0.82 0.14 60)"/>
      </div>

      <div className="grid grid-12 mb-12">
        <div className="card" style={{ gridColumn: "span 8" }}>
          <div className="card__head">
            <div className="card__title">Pools</div>
            <div className="card__sub">4 online</div>
            <div className="card__actions">
              <div className="seg">
                <button className="is-active">Capacity</button>
                <button>Throughput</button>
                <button>IOPS</button>
              </div>
            </div>
          </div>
          <div className="card__body card__body--flush">
            <table className="tbl">
              <thead><tr>
                <th>Name</th><th>Tier</th><th>Protection</th><th>Disks</th><th>Usage</th><th className="num">Read</th><th className="num">Write</th>
              </tr></thead>
              <tbody>
                {POOLS.map((p, i) => (
                  <tr key={p.name}>
                    <td><span className="sdot sdot--ok" style={{ marginRight: 8 }}/>
                      <span className="fg0" style={{ fontWeight: 500 }}>{p.name}</span>
                    </td>
                    <td><span className="chip">{p.tier}</span></td>
                    <td><span className="chip">{p.protection}</span></td>
                    <td className="mono">{p.disks}× {p.devices}</td>
                    <td><CapacityBar used={p.used} total={p.total}/></td>
                    <td className="num">{p.throughput.r} MB/s</td>
                    <td className="num">{p.throughput.w} MB/s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="card" style={{ gridColumn: "span 4" }}>
          <div className="card__head">
            <div className="card__title">Alerts & activity</div>
            <div className="card__actions">
              <button className="btn btn--ghost btn--sm">View all</button>
            </div>
          </div>
          <div className="card__body card__body--flush">
            <div className="feed">
              {ACTIVITY.map((a, i) => (
                <div className="feed__item" key={i}>
                  <StatusDot tone={a.tone}/>
                  <div className="feed__text">{a.text}</div>
                  <div className="feed__time">{a.t}</div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div className="grid grid-12">
        <div className="card" style={{ gridColumn: "span 4" }}>
          <div className="card__head">
            <div className="card__title">In-progress jobs</div>
            <div className="card__sub">3 running</div>
          </div>
          <div className="card__body col gap-12">
            <JobRow title="Scrub · pool bulk" sub="4.2 TB / 23 TB · 18% · 2h 40m" pct={0.18}/>
            <JobRow title="Replication · photos → offsite" sub="612 MB / 1.4 GB · 43% · 2m" pct={0.43} color="oklch(0.78 0.14 160)"/>
            <JobRow title="Snapshot prune · family-media" sub="schedule retention · running" pct={0.72} color="oklch(0.82 0.14 60)"/>
          </div>
        </div>
        <div className="card" style={{ gridColumn: "span 4" }}>
          <div className="card__head">
            <div className="card__title">Top datasets</div>
            <div className="card__sub">by growth 7d</div>
          </div>
          <div className="card__body card__body--flush">
            <table className="tbl">
              <tbody>
                {DATASETS.slice(0,5).map(d => (
                  <tr key={d.name}>
                    <td>
                      <div className="fg0">{d.name}</div>
                      <div className="mono muted" style={{ fontSize: 11 }}>{d.pool} · {d.proto}</div>
                    </td>
                    <td className="num">{fmtBytes(d.used)}</td>
                    <td className="num" style={{ color: "var(--ok)" }}>+{(Math.random()*4+0.2).toFixed(1)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card" style={{ gridColumn: "span 4" }}>
          <div className="card__head">
            <div className="card__title">System services</div>
            <div className="card__sub">novanas-system</div>
          </div>
          <div className="card__body col gap-8">
            {[
              ["novanas-api", "Running", "ok"], ["novanas-operators", "Running", "ok"],
              ["chunk-engine (SPDK)", "Running", "ok"], ["keycloak", "Running", "ok"],
              ["openbao", "Unsealed", "ok"], ["postgres", "Running", "ok"],
              ["novaedge", "Running", "ok"], ["prometheus / loki / tempo", "Running", "ok"],
            ].map(([n, s, t]) => (
              <div className="row" key={n} style={{ justifyContent: "space-between", padding: "2px 0" }}>
                <div className="row gap-8"><StatusDot tone={t}/><span className="mono" style={{ fontSize: 12 }}>{n}</span></div>
                <span className="muted mono" style={{ fontSize: 11 }}>{s}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}

function MetricCard({ label, value, unit, delta, up, data, color }) {
  return (
    <div className="metric">
      <div className="metric__label">{label}</div>
      <div className="metric__value">{value}<span className="unit">{unit}</span></div>
      <div className={`metric__delta ${up ? "metric__delta--up" : delta?.startsWith("-") ? "metric__delta--down" : ""}`}>
        {delta} <span className="muted">· 1h</span>
      </div>
      <div className="metric__spark">
        <Sparkline data={data} color={color} height={40}/>
      </div>
    </div>
  );
}

function JobRow({ title, sub, pct, color = "var(--accent)" }) {
  return (
    <div className="col gap-4">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <div className="fg0" style={{ fontSize: 12, fontWeight: 500 }}>{title}</div>
        <div className="mono muted" style={{ fontSize: 11 }}>{Math.round(pct*100)}%</div>
      </div>
      <div className="pbar"><div style={{ width: `${pct*100}%`, background: color }}/></div>
      <div className="muted" style={{ fontSize: 11 }}>{sub}</div>
    </div>
  );
}

window.DashboardScreen = DashboardScreen;
