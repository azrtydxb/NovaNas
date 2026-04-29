import { useEffect, useState } from "react";
import { useWM } from "../wm/store";
import { APPS } from "../wm/registry";
import { useAuth } from "../store/auth";

export function TopBar({ onLauncher, onPalette }: { onLauncher: () => void; onPalette: () => void }) {
  const windows = useWM((s) => s.windows);
  const activeId = useWM((s) => s.activeId);
  const focus = useWM((s) => s.focus);
  const toggleMinimize = useWM((s) => s.toggleMinimize);
  const auth = useAuth();
  const [time, setTime] = useState(formatTime());
  useEffect(() => {
    const t = setInterval(() => setTime(formatTime()), 1000 * 30);
    return () => clearInterval(t);
  }, []);

  return (
    <div className="topbar">
      <button className="topbar__menu" onClick={onLauncher} aria-label="Open launcher">
        ☰
      </button>
      <div className="topbar__brand">
        <span className="topbar__logo">N</span>
        <span className="topbar__name">NovaNAS</span>
        <span className="topbar__ver">2.0.0</span>
      </div>
      <span className="topbar__divider" />
      <div className="topbar__tasks">
        {windows.map((w) => {
          const def = APPS[w.appId];
          return (
            <button
              key={w.id}
              className={`tb-task${activeId === w.id && !w.minimized ? " is-on" : ""}`}
              onClick={() => (w.minimized ? toggleMinimize(w.id) : focus(w.id))}
            >
              {def.title}
            </button>
          );
        })}
      </div>
      <span className="topbar__spacer" />
      <button className="topbar__search" onClick={onPalette}>
        <span className="topbar__search-text">Search apps, settings…</span>
        <kbd className="topbar__kbd">⌘K</kbd>
      </button>
      <span className="topbar__user" title={auth.user?.profile?.preferred_username ?? ""}>
        {(auth.user?.profile?.preferred_username ?? "U").slice(0, 2).toUpperCase()}
      </span>
      <span className="topbar__clock">{time}</span>
    </div>
  );
}

function formatTime() {
  return new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}
