import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWM } from "../wm/store";
import { APPS } from "../wm/registry";
import { useAuth } from "../store/auth";
import { api } from "../api/client";
import { Icon } from "../components/Icon";

type Props = {
  onLauncher: () => void;
  onPalette: () => void;
  onBell: () => void;
  unreadCount: number;
};

type AlertItem = { fingerprint?: string; status?: { state?: string }; state?: string; labels?: Record<string, string> };

export function TopBar({ onLauncher, onPalette, onBell, unreadCount }: Props) {
  const windows = useWM((s) => s.windows);
  const activeId = useWM((s) => s.activeId);
  const focus = useWM((s) => s.focus);
  const toggleMinimize = useWM((s) => s.toggleMinimize);
  const open = useWM((s) => s.open);
  const auth = useAuth();
  const logout = useAuth((s) => s.logout);
  const [time, setTime] = useState(formatTime());
  const [userMenu, setUserMenu] = useState(false);

  useEffect(() => {
    const t = setInterval(() => setTime(formatTime()), 30_000);
    return () => clearInterval(t);
  }, []);

  // Critical alert triangle (only shown when at least one critical
  // alert is firing — matches the design's red-triangle indicator).
  const alerts = useQuery({
    queryKey: ["topbar", "alerts"],
    queryFn: () => api<AlertItem[]>("/api/v1/alerts"),
    refetchInterval: 30_000,
  });
  const list = Array.isArray(alerts.data) ? alerts.data : [];
  const critical = list.filter((a) => {
    const s = (a.labels?.severity ?? "").toLowerCase();
    const st = (a.status?.state ?? a.state ?? "").toLowerCase();
    const firing = st === "" || st === "active" || st === "firing";
    return firing && (s === "critical" || s === "error");
  }).length;

  const username = auth.user?.profile?.preferred_username ?? "user";

  return (
    <div className="topbar">
      <button className="topbar__menu" onClick={onLauncher} aria-label="Open launcher">
        <Icon name="apps" size={16} />
      </button>
      <div className="topbar__brand">
        <span className="topbar__logo">N</span>
        <span className="topbar__name">NovaNAS</span>
      </div>
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
      {critical > 0 && (
        <button
          className="topbar__icon topbar__icon--alert"
          onClick={() => open("alerts")}
          aria-label={`${critical} critical alerts`}
          title={`${critical} critical alert${critical === 1 ? "" : "s"} — open Alerts`}
        >
          <Icon name="warning" size={15} />
        </button>
      )}
      <button className="topbar__icon" onClick={onBell} aria-label="Notifications">
        <Icon name="bell" size={14} />
        {unreadCount > 0 && <span className="topbar__badge" />}
      </button>
      <button
        className="topbar__icon topbar__user-btn"
        onClick={() => setUserMenu((v) => !v)}
        title={username}
      >
        <Icon name="user" size={14} />
      </button>
      {userMenu && (
        <div className="user-menu" onMouseLeave={() => setUserMenu(false)}>
          <div className="user-menu__head">
            <div className="user-menu__name">{username}</div>
            {auth.user?.profile?.email && (
              <div className="user-menu__email muted">{String(auth.user.profile.email)}</div>
            )}
          </div>
          <button className="user-menu__item" onClick={() => { setUserMenu(false); }}>
            <Icon name="settings" size={11} /> Settings
          </button>
          <button className="user-menu__item" onClick={() => { setUserMenu(false); logout(); }}>
            <Icon name="power" size={11} /> Sign out
          </button>
        </div>
      )}
      <span className="topbar__clock">{time}</span>
    </div>
  );
}

function formatTime() {
  return new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}
