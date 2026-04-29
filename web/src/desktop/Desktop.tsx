import { useEffect, useState } from "react";
import { TopBar } from "./TopBar";
import { Dock } from "./Dock";
import { Launcher } from "./Launcher";
import { Palette } from "./Palette";
import { BellDrawer } from "./BellDrawer";
import { Window } from "../wm/Window";
import { useWM } from "../wm/store";
import { APPS } from "../wm/registry";
import { useTheme } from "../store/theme";
import { Tweaks } from "./Tweaks";

export function Desktop() {
  const windows = useWM((s) => s.windows);
  const open = useWM((s) => s.open);
  const [launcherOpen, setLauncherOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [bellOpen, setBellOpen] = useState(false);
  const variant = useTheme((s) => s.variant);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (e.key === "Escape") {
        setLauncherOpen(false);
        setPaletteOpen(false);
        setBellOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Allow apps to open other apps without going through the registry.
  // Used by Control Panel.
  useEffect(() => {
    const onOpen = (e: Event) => {
      const detail = (e as CustomEvent<string>).detail;
      if (detail && detail in APPS) open(detail as keyof typeof APPS);
    };
    window.addEventListener("nova:open-app", onOpen);
    return () => window.removeEventListener("nova:open-app", onOpen);
  }, [open]);

  return (
    <div className={`os os--${variant}`}>
      <div className="wallpaper" />
      <TopBar
        onLauncher={() => setLauncherOpen(true)}
        onPalette={() => setPaletteOpen(true)}
        onBell={() => setBellOpen((v) => !v)}
        unreadCount={0}
      />
      <div className="windows-layer">
        {windows.map((w) => {
          const def = APPS[w.appId];
          const Comp = def.Component;
          return (
            <Window key={w.id} state={w} title={def.title} icon={def.icon}>
              <Comp />
            </Window>
          );
        })}
      </div>
      <Dock />
      {launcherOpen && <Launcher onClose={() => setLauncherOpen(false)} />}
      {paletteOpen && <Palette onClose={() => setPaletteOpen(false)} />}
      {bellOpen && <BellDrawer onClose={() => setBellOpen(false)} />}
      <Tweaks />
    </div>
  );
}
