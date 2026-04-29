/* globals React, TweaksPanel, useTweaks, TweakSection, TweakRadio, TweakToggle, TweakSelect, TweakSlider, TweakColor */

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "aurora",
  "accent": "#5b9cff",
  "density": "comfortable",
  "wallpaper": "starfield",
  "monoEverywhere": false,
  "monoFont": "Geist Mono",
  "uiFont": "Geist",
  "showWidgets": true,
  "openOnBoot": "storage,alerts"
}/*EDITMODE-END*/;

const THEME_VARS = {
  aurora: {
    "--bg-0": "oklch(0.18 0.018 250)",
    "--bg-1": "oklch(0.22 0.018 250)",
    "--bg-2": "oklch(0.26 0.018 250)",
    "--bg-3": "oklch(0.30 0.020 250)",
  },
  graphite: {
    "--bg-0": "oklch(0.16 0 0)",
    "--bg-1": "oklch(0.20 0 0)",
    "--bg-2": "oklch(0.24 0 0)",
    "--bg-3": "oklch(0.28 0 0)",
  },
  aether: {
    "--bg-0": "oklch(0.96 0.005 250)",
    "--bg-1": "oklch(0.99 0.003 250)",
    "--bg-2": "oklch(0.97 0.005 250)",
    "--bg-3": "oklch(0.93 0.008 250)",
    "--fg-0": "oklch(0.20 0 0)",
    "--fg-1": "oklch(0.30 0 0)",
    "--fg-2": "oklch(0.45 0 0)",
    "--fg-3": "oklch(0.60 0 0)",
    "--line": "oklch(0.88 0 0)",
  },
};

function applyTheme(theme, accent, density) {
  const root = document.documentElement;
  // base theme vars
  Object.entries(THEME_VARS[theme] || THEME_VARS.aurora).forEach(([k,v]) => root.style.setProperty(k, v));
  // restore foreground for dark themes
  if (theme !== "aether") {
    root.style.setProperty("--fg-0", "oklch(0.95 0 0)");
    root.style.setProperty("--fg-1", "oklch(0.85 0 0)");
    root.style.setProperty("--fg-2", "oklch(0.65 0 0)");
    root.style.setProperty("--fg-3", "oklch(0.45 0 0)");
    root.style.setProperty("--line", "oklch(0.32 0.012 250)");
  }
  // accent
  if (accent) {
    root.style.setProperty("--accent", accent);
    // Manually compute soft variant — oklch from-string isn't always supported in all engines
    root.style.setProperty("--accent-soft", `${accent}22`);
  }
  // density
  const scale = density === "compact" ? 0.92 : density === "spacious" ? 1.08 : 1.0;
  root.style.setProperty("--density", scale);
}

function NovaTweaks() {
  const [t, setT] = useTweaks(TWEAK_DEFAULTS);

  // Apply on every change
  React.useEffect(() => {
    applyTheme(t.theme, t.accent, t.density);
    document.documentElement.style.setProperty("--font-mono", `"${t.monoFont}", ui-monospace, monospace`);
    document.documentElement.style.setProperty("--font", `"${t.uiFont}", ui-sans-serif, system-ui, sans-serif`);
    if (t.monoEverywhere) document.body.style.fontFamily = "var(--font-mono)";
    else document.body.style.fontFamily = "var(--font)";
    // Hide widgets
    const w = document.querySelector(".desktop-widgets");
    if (w) w.style.display = t.showWidgets ? "" : "none";
    // Wallpaper
    document.documentElement.setAttribute("data-wallpaper", t.wallpaper);
  }, [t]);

  return (
    <TweaksPanel title="Tweaks">
      <TweakSection label="Theme">
        <TweakRadio label="Variant" value={t.theme} onChange={v => setT("theme", v)} options={[
          {label: "Aurora", value: "aurora"},
          {label: "Graphite", value: "graphite"},
          {label: "Aether", value: "aether"},
        ]}/>
        <TweakColor label="Accent" value={t.accent} onChange={v => setT("accent", v)}/>
        <TweakRadio label="Density" value={t.density} onChange={v => setT("density", v)} options={[
          {label: "Compact", value: "compact"},
          {label: "Default", value: "comfortable"},
          {label: "Spacious", value: "spacious"},
        ]}/>
      </TweakSection>

      <TweakSection label="Typography">
        <TweakSelect label="UI font" value={t.uiFont} onChange={v => setT("uiFont", v)} options={[
          "Geist","Inter","IBM Plex Sans","System UI"
        ].map(f => ({label: f, value: f}))}/>
        <TweakSelect label="Mono font" value={t.monoFont} onChange={v => setT("monoFont", v)} options={[
          "Geist Mono","JetBrains Mono","IBM Plex Mono","ui-monospace"
        ].map(f => ({label: f, value: f}))}/>
        <TweakToggle label="Mono everywhere" value={t.monoEverywhere} onChange={v => setT("monoEverywhere", v)}/>
      </TweakSection>

      <TweakSection label="Desktop">
        <TweakRadio label="Wallpaper" value={t.wallpaper} onChange={v => setT("wallpaper", v)} options={[
          {label: "Stars", value: "starfield"},
          {label: "Aurora", value: "aurora-grad"},
          {label: "Solid", value: "solid"},
        ]}/>
        <TweakToggle label="Show widgets" value={t.showWidgets} onChange={v => setT("showWidgets", v)}/>
      </TweakSection>
    </TweaksPanel>
  );
}

window.NovaTweaks = NovaTweaks;
