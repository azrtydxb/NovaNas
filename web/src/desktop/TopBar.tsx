import { useEffect, useState } from "react";
import { useWM } from "../wm/store";
import { APPS } from "../wm/registry";
import { useAuth } from "../store/auth";
import { Icon } from "../components/Icon";

type Props = {
  onLauncher: () => void;
  onPalette: () => void;
  onBell: () => void;
  unreadCount: number;
};

export function TopBar({ onLauncher, onPalette, onBell, unreadCount }: Props) {
  const windows = useWM((s) => s.windows);
  const activeId = useWM((s) => s.activeId);
  const focus = useWM((s) => s.focus);
  const toggleMinimize = useWM((s) => s.toggleMinimize);
  const auth = useAuth();
  const [time, setTime] = useState(formatTime());
  useEffect(() => {
    const t = setInterval(() => setTime(formatTime()), 30_000);
    return () => clearInterval(t);
  }, []);

  const initials = (auth.user?.profile?.preferred_username ?? "U").slice(0, 2).toUpperCase();

  return (
    <div className="topbar">
      <button className="topbar__menu" onClick={onLauncher} aria-label="Open launcher">
        <Icon name="burger" size={16} />
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
          const on = activeId === w.id && !w.minimized;
          return (
            <button
              key={w.id}
              className={`tb-task${on ? " is-on" : ""}`}
              onClick={() => (w.minimized ? toggleMinimize(w.id) : focus(w.id))}
            >
              <Icon name={def.icon} size={11} />
              {def.title}
            </button>
          );
        })}
      </div>
      <span className="topbar__spacer" />
      <button className="topbar__search" onClick={onPalette}>
        <Icon name="search" size={12} />
        <span className="topbar__search-text">Search apps, settings, files…</span>
        <kbd className="topbar__kbd">⌘K</kbd>
      </button>
      <button className="topbar__icon" onClick={onBell} aria-label="Notifications">
        <Icon name="bell" size={14} />
        {unreadCount > 0 && <span className="topbar__badge" />}
      </button>
      <span className="topbar__user" title={auth.user?.profile?.preferred_username ?? ""}>
        {initials}
      </span>
      <span className="topbar__clock">{time}</span>
    </div>
  );
}

function formatTime() {
  return new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}
