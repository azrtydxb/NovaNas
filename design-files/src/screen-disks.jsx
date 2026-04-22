/* globals React, Icon, StatusDot, Pill, KV, fmtBytes, DISKS, SEEDS */
function DisksScreen({ seed }) {
  const disks = React.useMemo(() => {
    // Apply seed mutations
    if (seed === "rebuilding") {
      return DISKS.map(d => d.slot === 13 ? { ...d, state: "FAILED", reason: "I/O errors — replaced by spare" } : d.slot === 8 ? { ...d, state: "DRAINING", role: "data", pool: "fast", reason: "Rebuilding onto spare" } : d);
    }
    if (seed === "healthy") {
      return DISKS.map(d => d.slot === 13 ? { ...d, state: "ACTIVE", reason: undefined } : d);
    }
    return DISKS;
  }, [seed]);
  const [sel, setSel] = React.useState(13);
  const d = disks.find(x => x.slot === sel) || disks[0];

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Disks</h1>
          <div className="page-head__sub">Enclosure jbod-24 · 24 slots · SES auto-detected</div>
        </div>
        <div className="page-head__actions">
          <div className="seg">
            <button className="is-active">Enclosure</button>
            <button>List</button>
            <button>SMART</button>
          </div>
          <button className="btn"><Icon name="refresh" size={13}/> Rescan</button>
        </div>
      </div>

      <div className="split">
        <div className="col">
          <div className="encl">
            <div className="encl__title">
              <span>ENCLOSURE · JBOD-24</span>
              <span className="muted mono">6 × 4 slots</span>
            </div>
            <div className="encl__grid">
              {disks.map(x => (
                <div
                  key={x.slot}
                  className={`encl__slot ${x.state === "EMPTY" ? "is-empty" : ""} ${sel === x.slot ? "is-selected" : ""}`}
                  data-state={x.state}
                  onClick={() => x.state !== "EMPTY" && setSel(x.slot)}
                >
                  <div className="row" style={{ justifyContent: "space-between" }}>
                    <span className="num">#{String(x.slot).padStart(2,"0")}</span>
                    <span className="led"/>
                  </div>
                  {x.state !== "EMPTY" ? (
                    <>
                      <div className="model truncate">{x.model}</div>
                      <div className="row" style={{ justifyContent: "space-between" }}>
                        <span className="cap">{fmtBytes(x.cap, 0)}</span>
                        <span className="muted mono" style={{ fontSize: 10 }}>{x.pool || "—"}</span>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="model" style={{ color: "var(--fg-4)" }}>empty</div>
                      <div className="row" style={{ justifyContent: "flex-end" }}>
                        <span className="muted mono" style={{ fontSize: 10 }}>—</span>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
            <div className="row gap-12 muted" style={{ fontSize: 11 }}>
              <div className="row gap-4"><span className="sdot sdot--ok"/> Active</div>
              <div className="row gap-4"><span className="sdot sdot--warn"/> Degraded</div>
              <div className="row gap-4"><span className="sdot sdot--err"/> Failed</div>
              <div className="row gap-4"><span className="sdot sdot--info"/> Draining</div>
              <div className="row gap-4"><span className="sdot sdot--idle"/> Spare</div>
            </div>
          </div>
        </div>

        <aside className="drawer">
          <div className="drawer__head">
            <div>
              <div className="muted" style={{ fontSize: 11 }}>Slot #{d.slot}</div>
              <div className="fg0" style={{ fontWeight: 500, marginTop: 2 }}>{d.model || "Empty slot"}</div>
              {d.state && <div style={{ marginTop: 6 }}><Pill tone={d.state === "ACTIVE" ? "ok" : d.state === "DEGRADED" ? "warn" : d.state === "FAILED" ? "err" : "accent"} dot>{d.state}</Pill></div>}
            </div>
          </div>
          <div className="drawer__body">
            {d.state === "EMPTY" ? (
              <div className="muted">No disk in this slot. Hot-insert to begin.</div>
            ) : (
              <>
                <KV rows={[
                  ["WWN", d.wwn],
                  ["Pool", d.pool],
                  ["Role", d.role],
                  ["Class", d.class],
                  ["Capacity", fmtBytes(d.cap, 0)],
                  ["Temp", `${d.temp}°C`],
                  ["Power-on", `${d.hours.toLocaleString()} h`],
                ]}/>
                {d.reason && (
                  <div className="banner banner--warn">
                    <Icon name="alert" size={14}/>
                    <div className="banner__text">{d.reason}</div>
                  </div>
                )}
                <div className="col gap-8">
                  <div className="muted" style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.06em" }}>SMART</div>
                  <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>Reallocated</span><span className="mono fg0">{d.state === "DEGRADED" ? "5" : "0"}</span></div>
                  <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>Pending</span><span className="mono fg0">0</span></div>
                  <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>I/O errors (24h)</span><span className="mono fg0">{d.state === "DEGRADED" ? "12" : "0"}</span></div>
                </div>
                <div className="row gap-8">
                  <button className="btn btn--sm"><Icon name="zap" size={12}/> Blink LED</button>
                  <button className="btn btn--sm">Drain</button>
                  <button className="btn btn--sm btn--danger">Wipe</button>
                </div>
              </>
            )}
          </div>
        </aside>
      </div>
    </>
  );
}

window.DisksScreen = DisksScreen;
