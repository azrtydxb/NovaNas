/* globals React, Icon, Pill, StatusDot */
const { useState: useStateCh } = React;

const NAV_ADMIN = [
  { id: "dashboard", label: "Dashboard", icon: "dashboard" },
  { id: "pools",     label: "Storage",   icon: "storage", group: "storage",
    sub: [
      { id: "pools",    label: "Pools" },
      { id: "datasets", label: "Datasets" },
      { id: "disks",    label: "Disks" },
      { id: "snapshots", label: "Snapshots" },
    ]
  },
  { id: "shares",    label: "Sharing",   icon: "share", group: "sharing",
    sub: [
      { id: "shares",   label: "Shares" },
      { id: "iscsi",    label: "iSCSI / NVMe-oF" },
      { id: "s3",       label: "S3" },
    ]
  },
  { id: "protect",   label: "Data Protection", icon: "protect" },
  { id: "apps",      label: "Apps",       icon: "app", count: 8 },
  { id: "vms",       label: "VMs",        icon: "vm",  count: 6 },
  { id: "network",   label: "Network",    icon: "network" },
  { id: "identity",  label: "Identity",   icon: "identity" },
  { id: "system",    label: "System",     icon: "system" },
];

const NAV_USER = [
  { id: "dashboard",  label: "My Dashboard", icon: "dashboard" },
  { id: "datasets",   label: "My Datasets",  icon: "dataset" },
  { id: "shares",     label: "My Shares",    icon: "share" },
  { id: "snapshots",  label: "My Snapshots", icon: "snap" },
  { id: "apps",       label: "My Apps",      icon: "app" },
  { id: "vms",        label: "My VMs",       icon: "vm" },
];

function Brand() {
  return (
    <div className="brand">
      <div className="brand__mark">
        {/* subtle geometric glyph: stacked chunks */}
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round">
          <path d="M2 4.5L7 2l5 2.5L7 7z" fill="currentColor" fillOpacity="0.9" stroke="none"/>
          <path d="M2 7L7 9.5 12 7M2 9.5L7 12l5-2.5" />
        </svg>
      </div>
      <div className="brand__name">NovaNas</div>
      <span className="brand__box mono">nas-01</span>
    </div>
  );
}

function Topbar({ route, setRoute, role, setRole, onTweaks }) {
  return (
    <div className="topbar">
      <Brand/>
      <div className="breadcrumb">
        <span>Console</span>
        <span className="breadcrumb__sep">/</span>
        <span className="breadcrumb__cur">{routeLabel(route)}</span>
      </div>
      <div className="topbar__spacer"/>
      <div className="top-search">
        <Icon name="search" size={13}/>
        <input placeholder="Search datasets, apps, disks…"/>
        <kbd>⌘K</kbd>
      </div>
      <Pill tone="ok" dot>26.07.3</Pill>
      <div className="role-switch" role="tablist">
        <button className={role==="admin"?"is-active":""} onClick={()=>setRole("admin")}>
          <Icon name="shield" size={12}/> Admin
        </button>
        <button className={role==="user"?"is-active":""} onClick={()=>setRole("user")}>
          <Icon name="user" size={12}/> User
        </button>
      </div>
      <button className="top-icon-btn" title="Alerts">
        <Icon name="bell" size={15}/>
        <span className="badge"/>
      </button>
      <button className="top-icon-btn" title="Tweaks" onClick={onTweaks}>
        <Icon name="sliders" size={15}/>
      </button>
      <div className="user-chip">
        <div className="user-chip__avatar">PW</div>
        <div className="user-chip__name">pascal</div>
        <Icon name="chevDown" size={12} style={{ color: "var(--fg-3)", marginRight: 4 }}/>
      </div>
    </div>
  );
}

function Rail({ route, setRoute, role }) {
  const nav = role === "admin" ? NAV_ADMIN : NAV_USER;
  return (
    <aside className="rail">
      {nav.map(item => (
        <React.Fragment key={item.id}>
          <div
            className={`rail__item ${route === item.id ? "is-active" : ""}`}
            onClick={() => setRoute(item.id)}
          >
            <Icon name={item.icon} size={15}/>
            <span>{item.label}</span>
            {item.count != null && <span className="count">{item.count}</span>}
          </div>
          {item.sub && route.startsWith(item.id) && (
            <div className="rail__sub">
              {item.sub.map(s => (
                <div
                  key={s.id}
                  className={`rail__item ${route === s.id ? "is-active" : ""}`}
                  onClick={() => setRoute(s.id)}
                >
                  <span>{s.label}</span>
                </div>
              ))}
            </div>
          )}
        </React.Fragment>
      ))}
      <div className="rail__foot">
        <div className="row" style={{ justifyContent: "space-between" }}>
          <span>Capacity used</span>
          <span className="mono fg0">42.1 / 98.1 TB</span>
        </div>
        <div className="rail__bar"><div style={{ width: "43%" }}/></div>
        <div className="row" style={{ justifyContent: "space-between", opacity: 0.8 }}>
          <span>Uptime</span>
          <span className="mono">42d 11h</span>
        </div>
      </div>
    </aside>
  );
}

function routeLabel(r) {
  return ({
    dashboard: "Dashboard", pools: "Pools", datasets: "Datasets", disks: "Disks",
    snapshots: "Snapshots", shares: "Shares", iscsi: "iSCSI / NVMe-oF", s3: "S3",
    protect: "Data Protection", apps: "Apps", vms: "Virtual Machines",
    network: "Network", identity: "Identity", system: "System",
  }[r] || "Console");
}

window.Topbar = Topbar;
window.Rail = Rail;
window.routeLabel = routeLabel;
