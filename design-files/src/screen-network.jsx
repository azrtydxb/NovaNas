/* globals React, Icon, Pill */
function NetworkScreen() {
  const ifs = [
    { name: "bond0",    type: "LACP", slaves: "eno1+eno2", speed: "20 Gb/s", ip: "192.168.1.10/24", state: "up" },
    { name: "br0",      type: "bridge", slaves: "bond0", speed: "—",        ip: "—",              state: "up" },
    { name: "eno3",     type: "physical", slaves: "—",    speed: "1 Gb/s",  ip: "—",              state: "down" },
    { name: "vlan.100", type: "vlan", slaves: "bond0",   speed: "20 Gb/s",  ip: "10.0.100.10/24", state: "up" },
  ];
  return (
    <>
      <div className="page-head">
        <div><h1>Network</h1><div className="page-head__sub">4 interfaces · novanet eBPF · novaedge routing</div></div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="refresh" size={13}/> Rescan</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> Add VLAN</button>
        </div>
      </div>
      <div className="grid grid-12 mb-12">
        <div className="card" style={{ gridColumn: "span 8" }}>
          <div className="card__head"><div className="card__title">Interfaces</div></div>
          <table className="tbl">
            <thead><tr><th>Name</th><th>Type</th><th>Members</th><th>Speed</th><th>IP</th><th>State</th></tr></thead>
            <tbody>
              {ifs.map(i => (
                <tr key={i.name}>
                  <td className="mono fg0">{i.name}</td>
                  <td><span className="chip">{i.type}</span></td>
                  <td className="mono muted">{i.slaves}</td>
                  <td className="mono">{i.speed}</td>
                  <td className="mono">{i.ip}</td>
                  <td>{i.state === "up" ? <Pill tone="ok" dot>up</Pill> : <Pill tone="err" dot>down</Pill>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="card" style={{ gridColumn: "span 4" }}>
          <div className="card__head"><div className="card__title">Discovery</div></div>
          <div className="card__body col gap-8">
            {["mDNS (Avahi)", "SSDP", "WS-Discovery"].map(s => (
              <div key={s} className="row" style={{ justifyContent: "space-between", fontSize: 12 }}>
                <span className="mono">{s}</span>
                <Pill tone="ok" dot>active</Pill>
              </div>
            ))}
            <div className="hint mt-8">nas.local advertised on LAN.</div>
          </div>
        </div>
      </div>
    </>
  );
}
window.NetworkScreen = NetworkScreen;
