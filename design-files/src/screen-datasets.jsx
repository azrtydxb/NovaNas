/* globals React, Icon, Pill, CapacityBar, fmtBytes, DATASETS */
function DatasetsScreen({ role }) {
  const shown = role === "user" ? DATASETS.filter(d => d.owner.startsWith("pascal") || d.owner === "family") : DATASETS;
  return (
    <>
      <div className="page-head">
        <div>
          <h1>{role === "user" ? "My Datasets" : "Datasets"}</h1>
          <div className="page-head__sub">{shown.length} datasets · 4 pools</div>
        </div>
        <div className="page-head__actions">
          <input className="input" placeholder="Filter…" style={{ width: 220 }}/>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> New dataset</button>
        </div>
      </div>
      <div className="card">
        <table className="tbl">
          <thead><tr>
            <th>Name</th><th>Pool</th><th>Protocols</th><th>Owner</th><th>Protection</th>
            <th style={{ minWidth: 240 }}>Usage</th><th className="num">Snap.</th><th></th>
          </tr></thead>
          <tbody>
            {shown.map(d => (
              <tr key={d.name}>
                <td>
                  <div className="row gap-8">
                    <Icon name={d.proto === "bucket" ? "storage" : "dataset"} size={14} style={{ color: "var(--fg-3)" }}/>
                    <div>
                      <div className="fg0">{d.name}</div>
                      {d.enc && <div className="muted mono" style={{ fontSize: 10 }}><Icon name="lock" size={10}/> AES-256-GCM</div>}
                    </div>
                  </div>
                </td>
                <td><span className="chip">{d.pool}</span></td>
                <td>
                  {d.proto.split(" + ").map(p => <span key={p} className="chip" style={{ marginRight: 4 }}>{p}</span>)}
                </td>
                <td className="mono">{d.owner}</td>
                <td><span className="chip">{d.prot}</span></td>
                <td><CapacityBar used={d.used} total={d.quota}/></td>
                <td className="num">{d.snap}</td>
                <td><button className="btn btn--ghost btn--sm"><Icon name="more" size={12}/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
window.DatasetsScreen = DatasetsScreen;
