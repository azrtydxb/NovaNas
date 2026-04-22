/* globals React, Icon, Pill, StatusDot, Sparkline, CapacityBar, fmtBytes, POOLS, DISKS */
function PoolsScreen() {
  const [sel, setSel] = React.useState(POOLS[0].name);
  const pool = POOLS.find(p => p.name === sel);
  const diskInPool = DISKS.filter(d => d.pool === sel);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Pools</h1>
          <div className="page-head__sub">{POOLS.length} pools · {DISKS.filter(d=>d.state!=="EMPTY").length} disks active</div>
        </div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="refresh" size={13}/> Refresh</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> Create pool</button>
        </div>
      </div>

      <div className="grid grid-4 mb-12">
        {POOLS.map(p => (
          <div key={p.name} className="card" style={{ cursor: "pointer", borderColor: p.name === sel ? "var(--accent)" : undefined, boxShadow: p.name === sel ? "0 0 0 1px var(--accent)" : undefined }} onClick={() => setSel(p.name)}>
            <div className="card__head">
              <div className="card__title"><StatusDot tone="ok"/> <span style={{ marginLeft: 6 }}>{p.name}</span></div>
              <div className="card__sub"><span className="chip">{p.tier}</span></div>
            </div>
            <div className="card__body col gap-8">
              <div className="row" style={{ justifyContent: "space-between", fontSize: 11, color: "var(--fg-3)" }}>
                <span>Protection</span><span className="mono fg0">{p.protection}</span>
              </div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 11, color: "var(--fg-3)" }}>
                <span>Disks</span><span className="mono fg0">{p.disks}× {p.devices}</span>
              </div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 11, color: "var(--fg-3)" }}>
                <span>Read / Write</span><span className="mono fg0">{p.throughput.r} / {p.throughput.w} MB/s</span>
              </div>
              <CapacityBar used={p.used} total={p.total}/>
            </div>
          </div>
        ))}
      </div>

      <div className="card mb-12">
        <div className="card__head">
          <div className="card__title">Pool · {pool.name}</div>
          <div className="card__sub">{diskInPool.length} disks · {pool.protection}</div>
          <div className="card__actions">
            <button className="btn btn--sm"><Icon name="spark" size={12}/> Scrub now</button>
            <button className="btn btn--sm"><Icon name="edit" size={12}/> Edit</button>
            <button className="btn btn--sm"><Icon name="plus" size={12}/> Add disk</button>
          </div>
        </div>
        <div className="card__body card__body--flush">
          <table className="tbl">
            <thead><tr>
              <th>Slot</th><th>Model</th><th>Class</th><th>Role</th><th>State</th><th className="num">Capacity</th><th className="num">Temp</th><th className="num">Power-on hrs</th><th></th>
            </tr></thead>
            <tbody>
              {diskInPool.map(d => (
                <tr key={d.slot}>
                  <td className="mono">#{d.slot}</td>
                  <td>{d.model}</td>
                  <td><span className="chip">{d.class}</span></td>
                  <td><span className="chip">{d.role}</span></td>
                  <td>{
                    d.state === "ACTIVE" ? <span className="pill pill--ok"><span className="dot"/>ACTIVE</span>
                    : d.state === "DEGRADED" ? <span className="pill pill--warn"><span className="dot"/>DEGRADED</span>
                    : d.state === "SPARE" ? <span className="pill"><span className="dot"/>SPARE</span>
                    : <span className="pill">{d.state}</span>
                  }</td>
                  <td className="num">{fmtBytes(d.cap, 0)}</td>
                  <td className="num" style={{ color: d.temp > 46 ? "var(--warn)" : "var(--fg-1)" }}>{d.temp}°C</td>
                  <td className="num">{d.hours.toLocaleString()}</td>
                  <td><button className="btn btn--ghost btn--sm"><Icon name="more" size={12}/></button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

window.PoolsScreen = PoolsScreen;
