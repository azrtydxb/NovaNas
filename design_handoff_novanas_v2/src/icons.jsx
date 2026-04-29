/* globals React */

const Icon = ({ name, size = 16, sw = 1.8, style }) => {
  const p = ICONS[name];
  if (!p) return null;
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={sw} strokeLinecap="round" strokeLinejoin="round" style={style} aria-hidden="true">
      {p}
    </svg>
  );
};

const ICONS = {
  storage: (<><ellipse cx="12" cy="5" rx="9" ry="2.5"/><path d="M3 5v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5V5"/><path d="M3 11v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5v-6"/></>),
  apps: (<><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></>),
  files: (<><path d="M4 4h7l2 2h7v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"/></>),
  vm: (<><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></>),
  control: (<><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/></>),
  monitor: (<><path d="M3 12h4l3-8 4 16 3-8h4"/></>),
  search: (<><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></>),
  bell: (<><path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9z"/><path d="M10 21a2 2 0 0 0 4 0"/></>),
  user: (<><circle cx="12" cy="8" r="4"/><path d="M4 21c1-4 4.5-6 8-6s7 2 8 6"/></>),
  grid: (<><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></>),
  min: (<><path d="M5 12h14"/></>),
  max: (<><rect x="5" y="5" width="14" height="14" rx="1"/></>),
  close: (<><path d="M6 6l12 12M18 6L6 18"/></>),
  chev: (<><path d="M9 6l6 6-6 6"/></>),
  plus: (<><path d="M12 5v14M5 12h14"/></>),
  cpu: (<><rect x="6" y="6" width="12" height="12" rx="1.5"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/></>),
  ram: (<><rect x="2" y="8" width="20" height="10" rx="1.5"/><path d="M6 8v10M10 8v10M14 8v10M18 8v10M2 6h20"/></>),
  net: (<><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 3 2.5 15 0 18M12 3c-2.5 3-2.5 15 0 18"/></>),
  shield: (<><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M9 12l2 2 4-4"/></>),
  folder: (<><path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></>),
  doc: (<><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/></>),
  image: (<><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></>),
  video: (<><rect x="3" y="5" width="14" height="14" rx="2"/><path d="M21 7l-4 4 4 4z"/></>),
  terminal: (<><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3M13 15h4"/></>),
  power: (<><path d="M12 3v9"/><path d="M5.6 6.6a8 8 0 1 0 12.8 0"/></>),
  refresh: (<><path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v5h-5"/></>),
  more: (<><circle cx="5" cy="12" r="1.3" fill="currentColor"/><circle cx="12" cy="12" r="1.3" fill="currentColor"/><circle cx="19" cy="12" r="1.3" fill="currentColor"/></>),
  play: (<><path d="M6 4l14 8-14 8z" fill="currentColor" stroke="none"/></>),
  pause: (<><rect x="6" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/><rect x="14" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/></>),
  bolt: (<><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></>),
  globe: (<><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/></>),
  alert: (<><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18.5v.01"/></>),
  check: (<><path d="M5 12l5 5 9-11"/></>),
  download: (<><path d="M12 3v13m0 0l-4-4m4 4l4-4M4 21h16"/></>),
  log: (<><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9h10M7 13h10M7 17h6"/></>),
  warning: (<><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18.5v.01"/></>),
  filter: (<><path d="M3 5h18l-7 9v6l-4-2v-4z"/></>),
};

window.Icon = Icon;
