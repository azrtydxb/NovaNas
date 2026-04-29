import type { CSSProperties, ReactNode } from "react";

export type IconName =
  | "storage" | "apps" | "files" | "vm" | "control" | "monitor"
  | "search" | "bell" | "user" | "grid" | "min" | "max" | "close"
  | "chev" | "plus" | "cpu" | "ram" | "net" | "shield" | "folder"
  | "doc" | "image" | "video" | "terminal" | "power" | "refresh"
  | "more" | "play" | "pause" | "bolt" | "globe" | "alert" | "check"
  | "download" | "log" | "warning" | "filter"
  // Added beyond the design's icons.jsx for app needs:
  | "package" | "share" | "snapshot" | "replication" | "audit"
  | "job" | "settings" | "key" | "smb" | "nfs" | "iscsi" | "nvmeof"
  | "rdma" | "kerberos" | "workload" | "edit" | "trash" | "burger"
  | "stop" | "external" | "info" | "x" | "menu" | "lock" | "unlock";

const I: Record<IconName, ReactNode> = {
  storage: <><ellipse cx="12" cy="5" rx="9" ry="2.5"/><path d="M3 5v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5V5"/><path d="M3 11v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5v-6"/></>,
  apps: <><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></>,
  files: <path d="M4 4h7l2 2h7v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"/>,
  vm: <><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></>,
  control: <><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/></>,
  monitor: <path d="M3 12h4l3-8 4 16 3-8h4"/>,
  search: <><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></>,
  bell: <><path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9z"/><path d="M10 21a2 2 0 0 0 4 0"/></>,
  user: <><circle cx="12" cy="8" r="4"/><path d="M4 21c1-4 4.5-6 8-6s7 2 8 6"/></>,
  grid: <><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></>,
  min: <path d="M5 12h14"/>,
  max: <rect x="5" y="5" width="14" height="14" rx="1"/>,
  close: <path d="M6 6l12 12M18 6L6 18"/>,
  x: <path d="M6 6l12 12M18 6L6 18"/>,
  chev: <path d="M9 6l6 6-6 6"/>,
  plus: <path d="M12 5v14M5 12h14"/>,
  cpu: <><rect x="6" y="6" width="12" height="12" rx="1.5"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/></>,
  ram: <><rect x="2" y="8" width="20" height="10" rx="1.5"/><path d="M6 8v10M10 8v10M14 8v10M18 8v10M2 6h20"/></>,
  net: <><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 3 2.5 15 0 18M12 3c-2.5 3-2.5 15 0 18"/></>,
  shield: <><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M9 12l2 2 4-4"/></>,
  folder: <path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>,
  doc: <><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/></>,
  image: <><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></>,
  video: <><rect x="3" y="5" width="14" height="14" rx="2"/><path d="M21 7l-4 4 4 4z"/></>,
  terminal: <><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3M13 15h4"/></>,
  power: <><path d="M12 3v9"/><path d="M5.6 6.6a8 8 0 1 0 12.8 0"/></>,
  refresh: <><path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v5h-5"/></>,
  more: <><circle cx="5" cy="12" r="1.3" fill="currentColor"/><circle cx="12" cy="12" r="1.3" fill="currentColor"/><circle cx="19" cy="12" r="1.3" fill="currentColor"/></>,
  play: <path d="M6 4l14 8-14 8z" fill="currentColor" stroke="none"/>,
  pause: <><rect x="6" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/><rect x="14" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/></>,
  stop: <rect x="5" y="5" width="14" height="14" rx="2" fill="currentColor" stroke="none"/>,
  bolt: <path d="M13 2L4 14h7l-1 8 9-12h-7z"/>,
  globe: <><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/></>,
  alert: <><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18.5v.01"/></>,
  warning: <><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18.5v.01"/></>,
  check: <path d="M5 12l5 5 9-11"/>,
  download: <path d="M12 3v13m0 0l-4-4m4 4l4-4M4 21h16"/>,
  log: <><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9h10M7 13h10M7 17h6"/></>,
  filter: <path d="M3 5h18l-7 9v6l-4-2v-4z"/>,
  package: <><path d="M21 8l-9-5-9 5 9 5 9-5z"/><path d="M3 8v8l9 5 9-5V8"/><path d="M12 13v8"/></>,
  share: <><circle cx="6" cy="12" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 11l8-4M8 13l8 4"/></>,
  snapshot: <><circle cx="12" cy="12" r="3"/><path d="M21 12c0 5-4 9-9 9s-9-4-9-9 4-9 9-9c2 0 4 .7 6 2"/><path d="M17 3v4h4"/></>,
  replication: <><path d="M3 7h11l-3-3M21 17H10l3 3"/><path d="M3 7l3 3M21 17l-3-3"/></>,
  audit: <><rect x="4" y="3" width="14" height="18" rx="2"/><path d="M8 7h6M8 11h6M8 15h4"/><path d="M19 10v9a2 2 0 0 0 2-2"/></>,
  job: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
  settings: <><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></>,
  key: <><circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M16 7l3 3M19 4l3 3"/></>,
  smb: <><rect x="3" y="4" width="18" height="14" rx="2"/><path d="M7 20h10M12 18v2"/></>,
  nfs: <><path d="M3 12c0-5 4-9 9-9 4 0 7 2 8 5"/><path d="M21 12c0 5-4 9-9 9-4 0-7-2-8-5"/><path d="M3 8h6M21 16h-6"/></>,
  iscsi: <><rect x="3" y="6" width="18" height="12" rx="2"/><path d="M8 6v12M16 6v12M3 12h18"/></>,
  nvmeof: <><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 12h2M12 12h2M18 12h2"/></>,
  rdma: <><circle cx="6" cy="12" r="3"/><circle cx="18" cy="12" r="3"/><path d="M9 12h6"/></>,
  kerberos: <><path d="M12 3l9 4v5c0 5-4 8-9 9-5-1-9-4-9-9V7z"/><path d="M9 11l2 2 4-4"/></>,
  workload: <><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/><path d="M10 6.5h4M6.5 10v4M14 17.5h-4M17.5 14v-4" stroke-dasharray="2 1.5"/></>,
  edit: <><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4z"/></>,
  trash: <><path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M5 6l1 14a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2l1-14"/></>,
  burger: <path d="M3 6h18M3 12h18M3 18h18"/>,
  menu: <path d="M3 6h18M3 12h18M3 18h18"/>,
  external: <><path d="M14 3h7v7"/><path d="M21 3l-9 9"/><path d="M19 14v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h6"/></>,
  info: <><circle cx="12" cy="12" r="9"/><path d="M12 8v0.01M11 12h1v5h1"/></>,
  lock: <><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></>,
  unlock: <><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0"/></>,
};

type Props = {
  name: IconName;
  size?: number;
  sw?: number;
  style?: CSSProperties;
  className?: string;
};

export function Icon({ name, size = 16, sw = 1.8, style, className }: Props) {
  const path = I[name];
  if (!path) return null;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={sw}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={style}
      className={className}
      aria-hidden="true"
    >
      {path}
    </svg>
  );
}
