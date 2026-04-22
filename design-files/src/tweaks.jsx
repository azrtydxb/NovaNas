/* globals React, Icon, Pill */
function TweaksPanel({ tweaks, setTweaks, open, setOpen }) {
  if (!open) return null;
  const setKey = (k, v) => {
    const next = { ...tweaks, [k]: v };
    setTweaks(next);
    try {
      window.parent.postMessage({ type: "__edit_mode_set_keys", edits: { [k]: v } }, "*");
    } catch (e) {}
  };
  const hues = [
    { name: "cyan",   h: 220 },
    { name: "violet", h: 280 },
    { name: "green",  h: 150 },
    { name: "amber",  h: 70  },
    { name: "rose",   h: 10  },
  ];
  return (
    <div className="tweaks">
      <div className="tweaks__head">
        <Icon name="sliders" size={14}/>
        <span>Tweaks</span>
        <button className="btn btn--ghost btn--sm" style={{ marginLeft: "auto" }} onClick={() => setOpen(false)}>
          <Icon name="close" size={12}/>
        </button>
      </div>
      <div className="tweaks__body">
        <div className="tweaks__row">
          <label>Theme</label>
          <div className="seg">
            <button className={tweaks.theme === "dark" ? "is-active" : ""} onClick={() => setKey("theme", "dark")}>
              <Icon name="moon" size={11}/> Dark
            </button>
            <button className={tweaks.theme === "light" ? "is-active" : ""} onClick={() => setKey("theme", "light")}>
              <Icon name="sun" size={11}/> Light
            </button>
          </div>
        </div>
        <div className="tweaks__row">
          <label>Density</label>
          <div className="seg">
            <button className={tweaks.density === "dense" ? "is-active" : ""} onClick={() => setKey("density", "dense")}>Dense</button>
            <button className={tweaks.density === "spacious" ? "is-active" : ""} onClick={() => setKey("density", "spacious")}>Spacious</button>
          </div>
        </div>
        <div className="tweaks__row">
          <label>Accent</label>
          <div className="tweaks__swatches">
            {hues.map(h => (
              <button key={h.h}
                className={tweaks.accentHue === h.h ? "is-active" : ""}
                title={h.name}
                style={{ background: `oklch(0.78 0.14 ${h.h})` }}
                onClick={() => setKey("accentHue", h.h)}
              />
            ))}
          </div>
        </div>
        <div className="tweaks__row">
          <label>Role</label>
          <div className="seg">
            <button className={tweaks.role === "admin" ? "is-active" : ""} onClick={() => setKey("role", "admin")}>Admin</button>
            <button className={tweaks.role === "user" ? "is-active" : ""} onClick={() => setKey("role", "user")}>User</button>
          </div>
        </div>
        <div className="tweaks__row">
          <label>Seed data</label>
          <div className="seg">
            <button className={tweaks.seed === "healthy" ? "is-active" : ""} onClick={() => setKey("seed", "healthy")}>Healthy</button>
            <button className={tweaks.seed === "degraded" ? "is-active" : ""} onClick={() => setKey("seed", "degraded")}>Degraded</button>
            <button className={tweaks.seed === "rebuilding" ? "is-active" : ""} onClick={() => setKey("seed", "rebuilding")}>Rebuilding</button>
          </div>
        </div>
      </div>
    </div>
  );
}
window.TweaksPanel = TweaksPanel;
