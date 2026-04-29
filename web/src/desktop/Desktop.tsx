import { useEffect, useState } from "react";
import { TopBar } from "./TopBar";
import { Dock } from "./Dock";
import { Launcher } from "./Launcher";
import { Palette } from "./Palette";
import { Window } from "../wm/Window";
import { useWM } from "../wm/store";
import { APPS } from "../wm/registry";

export function Desktop() {
  const windows = useWM((s) => s.windows);
  const [launcherOpen, setLauncherOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (e.key === "Escape") {
        setLauncherOpen(false);
        setPaletteOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="os os--aurora">
      <div className="wallpaper" />
      <TopBar onLauncher={() => setLauncherOpen(true)} onPalette={() => setPaletteOpen(true)} />
      <div className="windows-layer">
        {windows.map((w) => {
          const def = APPS[w.appId];
          const Comp = def.Component;
          return (
            <Window key={w.id} state={w} title={def.title}>
              <Comp />
            </Window>
          );
        })}
      </div>
      <Dock />
      {launcherOpen && <Launcher onClose={() => setLauncherOpen(false)} />}
      {paletteOpen && <Palette onClose={() => setPaletteOpen(false)} />}
    </div>
  );
}
