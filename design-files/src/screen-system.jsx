/* globals React, Icon, Pill */
function SystemScreen() {
  return (
    <>
      <div className="page-head">
        <div><h1>System</h1><div className="page-head__sub">Appliance configuration</div></div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="download" size={13}/> Config backup</button>
          <button className="btn btn--danger"><Icon name="alert" size={13}/> Reboot</button>
        </div>
      </div>
      <div className="grid grid-3">
        {[
          ["Updates", "Up to date", "26.07.3 · checked 10 min ago", "ok"],
          ["Certificates", "4 active", "Wildcard *.nas.local · 87 days", "ok"],
          ["Alerts", "3 channels", "Email · ntfy · browser push", "ok"],
          ["Audit log", "1,204 events today", "Loki + syslog sinks", "ok"],
          ["Support", "Diagnostics ready", "Last bundle 2d ago", "ok"],
          ["TPM unseal", "Sealed & bound", "Auto-unseal on boot", "ok"],
        ].map(([t, v, s, tone]) => (
          <div key={t} className="card">
            <div className="card__body col gap-8">
              <div className="row" style={{ justifyContent: "space-between" }}>
                <div className="fg0" style={{ fontWeight: 500 }}>{t}</div>
                <Pill tone={tone} dot>{v}</Pill>
              </div>
              <div className="muted" style={{ fontSize: 12 }}>{s}</div>
              <button className="btn btn--sm" style={{ alignSelf: "flex-start", marginTop: 4 }}>Open</button>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
window.SystemScreen = SystemScreen;
