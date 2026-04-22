/* globals React, Icon, Pill, APPS_CATALOG */
function AppsScreen({ role }) {
  const [q, setQ] = React.useState("");
  const [cat, setCat] = React.useState("All");
  const [sel, setSel] = React.useState("immich");
  const cats = ["All", ...Array.from(new Set(APPS_CATALOG.map(a => a.cat)))];
  const filtered = APPS_CATALOG.filter(a =>
    (cat === "All" || a.cat === cat) &&
    (q === "" || a.name.toLowerCase().includes(q.toLowerCase()))
  );
  const app = APPS_CATALOG.find(a => a.slug === sel) || APPS_CATALOG[0];

  return (
    <>
      <div className="page-head">
        <div>
          <h1>{role === "user" ? "My Apps" : "Apps"}</h1>
          <div className="page-head__sub">Official catalog · {APPS_CATALOG.length} available · 8 installed</div>
        </div>
        <div className="page-head__actions">
          <div className="seg">
            <button className="is-active">Catalog</button>
            <button>Installed (8)</button>
            <button>Custom charts</button>
          </div>
          <button className="btn"><Icon name="refresh" size={13}/> Refresh catalog</button>
        </div>
      </div>

      <div className="row mb-12" style={{ gap: 8 }}>
        <div className="top-search" style={{ width: 320, background: "var(--bg-2)" }}>
          <Icon name="search" size={13}/>
          <input placeholder="Search apps…" value={q} onChange={e => setQ(e.target.value)}/>
        </div>
        <div className="seg">
          {cats.map(c => (
            <button key={c} className={cat === c ? "is-active" : ""} onClick={() => setCat(c)}>{c}</button>
          ))}
        </div>
        <div className="stretch"/>
        <Pill tone="ok" dot>signed by novanas-official</Pill>
      </div>

      <div className="grid" style={{ gridTemplateColumns: "1fr 380px", gap: 14 }}>
        <div className="grid grid-3" style={{ alignContent: "start" }}>
          {filtered.map(a => {
            const initials = a.name.split(/[^A-Za-z0-9]/).filter(Boolean).slice(0,2).map(s => s[0]).join("").toUpperCase();
            return (
              <div
                key={a.slug}
                className={`appcard ${sel === a.slug ? "is-selected" : ""}`}
                onClick={() => setSel(a.slug)}
              >
                <div className="appcard__row">
                  <div className="appcard__icon" style={{ "--grad": `linear-gradient(135deg, ${a.color}, oklch(from ${a.color} calc(l - 0.1) c calc(h + 40)))` }}>{initials}</div>
                  <div style={{ minWidth: 0 }}>
                    <div className="appcard__name truncate">{a.name}</div>
                    <div className="appcard__cat">{a.cat}</div>
                  </div>
                  {a.installed && <span className="pill pill--ok" style={{ marginLeft: "auto" }}><span className="dot"/>installed</span>}
                </div>
                <div className="appcard__desc">{a.desc}</div>
                <div className="appcard__tags">
                  {a.tags.map(t => <span key={t} className="chip">{t}</span>)}
                </div>
              </div>
            );
          })}
        </div>

        {/* Detail panel */}
        <aside className="card" style={{ position: "sticky", top: 16, alignSelf: "start" }}>
          <div className="card__body col gap-12">
            <div className="row gap-12">
              <div className="appcard__icon" style={{ width: 64, height: 64, borderRadius: 14, fontSize: 22, "--grad": `linear-gradient(135deg, ${app.color}, oklch(from ${app.color} calc(l - 0.1) c calc(h + 40)))` }}>
                {app.name.split(/[^A-Za-z0-9]/).filter(Boolean).slice(0,2).map(s => s[0]).join("").toUpperCase()}
              </div>
              <div>
                <div className="fg0" style={{ fontSize: 16, fontWeight: 600 }}>{app.name}</div>
                <div className="muted mono" style={{ fontSize: 11 }}>{app.cat} · v1.40.3 · 142 MB chart</div>
                <div style={{ marginTop: 6 }} className="row gap-4">
                  {app.tags.map(t => <span key={t} className="chip">{t}</span>)}
                </div>
              </div>
            </div>

            <p className="fg2" style={{ margin: 0, fontSize: 13, lineHeight: 1.55 }}>
              {app.desc} Renders as a Helm chart into your namespace with Pod Security Admission <span className="mono fg0">restricted</span>. Uses datasets you own for config and media.
            </p>

            <div className="col gap-4">
              <div className="muted" style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.06em" }}>Requirements</div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>Min RAM</span><span className="mono fg0">2048 MiB</span></div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>Ports</span><span className="mono fg0">32400/TCP</span></div>
              <div className="row" style={{ justifyContent: "space-between", fontSize: 12 }}><span>GPU</span><span className="mono fg0">optional</span></div>
            </div>

            <div className="col gap-4">
              <div className="muted" style={{ fontSize: 10, textTransform: "uppercase", letterSpacing: "0.06em" }}>Exposure</div>
              <div className="seg">
                <button className="is-active">reverseProxy</button>
                <button>mdns</button>
                <button>lan</button>
              </div>
              <div className="hint">→ <span className="mono">{app.slug}.nas.local</span></div>
            </div>

            {app.installed ? (
              <div className="row gap-8">
                <button className="btn btn--primary stretch"><Icon name="play" size={12}/> Open</button>
                <button className="btn"><Icon name="edit" size={12}/></button>
                <button className="btn"><Icon name="more" size={12}/></button>
              </div>
            ) : (
              <div className="row gap-8">
                <button className="btn btn--primary stretch"><Icon name="download" size={12}/> Install</button>
                <button className="btn"><Icon name="book" size={12}/> Docs</button>
              </div>
            )}

            {app.installed && (
              <div className="banner banner--warn">
                <Icon name="arrowUp" size={14}/>
                <div className="banner__text">
                  <div className="banner__title">Update available · 1.40.3 → 1.41.0</div>
                  <div style={{ color: "var(--fg-2)", fontSize: 11, marginTop: 2 }}>Pre-update snapshot will be taken automatically.</div>
                </div>
                <button className="btn btn--sm">Update</button>
              </div>
            )}
          </div>
        </aside>
      </div>
    </>
  );
}
window.AppsScreen = AppsScreen;
