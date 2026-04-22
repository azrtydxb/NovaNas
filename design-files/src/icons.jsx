/* globals React */
const { useState, useEffect, useRef, useMemo, useCallback } = React;

// ───────── icons (lucide-style, hand-picked for our surface) ─────────
const Icon = ({ name, size = 16, strokeWidth = 1.8, style }) => {
  const paths = ICONS[name];
  if (!paths) return null;
  return (
    <svg
      width={size} height={size} viewBox="0 0 24 24"
      fill="none" stroke="currentColor" strokeWidth={strokeWidth}
      strokeLinecap="round" strokeLinejoin="round"
      style={style} aria-hidden="true"
    >
      {paths}
    </svg>
  );
};

const ICONS = {
  dashboard: (<>
    <rect x="3" y="3" width="8" height="10" rx="1.5"/>
    <rect x="13" y="3" width="8" height="6" rx="1.5"/>
    <rect x="13" y="11" width="8" height="10" rx="1.5"/>
    <rect x="3" y="15" width="8" height="6" rx="1.5"/>
  </>),
  storage: (<>
    <ellipse cx="12" cy="5" rx="9" ry="2.5"/>
    <path d="M3 5v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5V5"/>
    <path d="M3 11v6c0 1.4 4 2.5 9 2.5s9-1.1 9-2.5v-6"/>
  </>),
  disk: (<>
    <circle cx="12" cy="12" r="9"/>
    <circle cx="12" cy="12" r="3"/>
    <path d="M12 3v3M12 18v3M3 12h3M18 12h3"/>
  </>),
  dataset: (<>
    <path d="M4 6h16v4H4zM4 14h16v4H4z"/>
    <circle cx="7" cy="8" r="0.6" fill="currentColor"/>
    <circle cx="7" cy="16" r="0.6" fill="currentColor"/>
  </>),
  share: (<>
    <path d="M4 4h7l2 2h7v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"/>
  </>),
  protect: (<>
    <path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/>
  </>),
  app: (<>
    <rect x="3" y="3" width="7" height="7" rx="1.5"/>
    <rect x="14" y="3" width="7" height="7" rx="1.5"/>
    <rect x="3" y="14" width="7" height="7" rx="1.5"/>
    <rect x="14" y="14" width="7" height="7" rx="1.5"/>
  </>),
  vm: (<>
    <rect x="2" y="4" width="20" height="13" rx="2"/>
    <path d="M8 21h8M12 17v4"/>
  </>),
  network: (<>
    <circle cx="12" cy="12" r="9"/>
    <path d="M3 12h18M12 3c2.5 3 2.5 15 0 18M12 3c-2.5 3-2.5 15 0 18"/>
  </>),
  identity: (<>
    <circle cx="12" cy="8" r="4"/>
    <path d="M4 21c1-4 4.5-6 8-6s7 2 8 6"/>
  </>),
  system: (<>
    <circle cx="12" cy="12" r="3"/>
    <path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>
  </>),
  search: (<><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></>),
  bell: (<>
    <path d="M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9z"/>
    <path d="M10 21a2 2 0 0 0 4 0"/>
  </>),
  plus: (<><path d="M12 5v14M5 12h14"/></>),
  download: (<><path d="M12 3v13m0 0l-4-4m4 4l4-4M4 21h16"/></>),
  play: (<><path d="M6 4l14 8-14 8z" fill="currentColor" stroke="none"/></>),
  pause: (<><rect x="6" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/><rect x="14" y="4" width="4" height="16" rx="1" fill="currentColor" stroke="none"/></>),
  stop: (<><rect x="5" y="5" width="14" height="14" rx="1" fill="currentColor" stroke="none"/></>),
  restart: (<><path d="M21 12a9 9 0 1 1-3-6.7M21 3v5h-5"/></>),
  check: (<><path d="M5 12l5 5 9-11"/></>),
  x: (<><path d="M6 6l12 12M18 6L6 18"/></>),
  chevRight: (<><path d="M9 6l6 6-6 6"/></>),
  chevDown: (<><path d="M6 9l6 6 6-6"/></>),
  chevUp: (<><path d="M6 15l6-6 6 6"/></>),
  arrowUp: (<><path d="M12 19V5M5 12l7-7 7 7"/></>),
  arrowDown: (<><path d="M12 5v14M19 12l-7 7-7-7"/></>),
  more: (<><circle cx="5" cy="12" r="1.3" fill="currentColor"/><circle cx="12" cy="12" r="1.3" fill="currentColor"/><circle cx="19" cy="12" r="1.3" fill="currentColor"/></>),
  filter: (<><path d="M3 4h18l-7 9v6l-4 2v-8z"/></>),
  refresh: (<><path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v5h-5"/></>),
  terminal: (<><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3M13 15h4"/></>),
  monitor: (<><rect x="2" y="4" width="20" height="14" rx="2"/><path d="M8 22h8M12 18v4"/></>),
  cpu: (<><rect x="6" y="6" width="12" height="12" rx="1.5"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2"/></>),
  ram: (<><rect x="2" y="8" width="20" height="10" rx="1.5"/><path d="M6 8v10M10 8v10M14 8v10M18 8v10M2 6h20"/></>),
  bolt: (<><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></>),
  snap: (<><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4"/></>),
  backup: (<><path d="M12 3v12M7 10l5 5 5-5"/><path d="M4 21h16"/></>),
  globe: (<><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/></>),
  key: (<><circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M15 5l3 3M17 3l3 3"/></>),
  lock: (<><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></>),
  spark: (<><path d="M12 3v3M5 6l2 2M19 6l-2 2M3 12h3M18 12h3M12 9l2 4-4 2 3 5"/></>),
  edit: (<><path d="M12 20h9M16.5 3.5a2 2 0 0 1 3 3L7 19l-4 1 1-4z"/></>),
  trash: (<><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/></>),
  console: (<><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M2 21h20M8 8l3 3-3 3"/></>),
  shield: (<><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M9 12l2 2 4-4"/></>),
  circle: (<><circle cx="12" cy="12" r="9"/></>),
  sliders: (<><path d="M4 6h12M4 12h6M4 18h10M18 4v4M14 10v4M16 16v4"/></>),
  sun: (<><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M2 12h2M20 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4"/></>),
  moon: (<><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></>),
  tag: (<><path d="M3 12V3h9l9 9-9 9z"/><circle cx="7.5" cy="7.5" r="1.5" fill="currentColor" stroke="none"/></>),
  zap: (<><path d="M13 2L4 14h7l-1 8 9-12h-7z" fill="currentColor" stroke="none" opacity="0.15"/><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></>),
  copy: (<><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M4 16V6a2 2 0 0 1 2-2h10"/></>),
  link: (<><path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/></>),
  activity: (<><path d="M3 12h4l3-8 4 16 3-8h4"/></>),
  server: (<><rect x="3" y="4" width="18" height="7" rx="1.5"/><rect x="3" y="13" width="18" height="7" rx="1.5"/><circle cx="7" cy="7.5" r="0.6" fill="currentColor"/><circle cx="7" cy="16.5" r="0.6" fill="currentColor"/></>),
  user: (<><circle cx="12" cy="8" r="4"/><path d="M4 21c1-4 4.5-6 8-6s7 2 8 6"/></>),
  book: (<><path d="M4 4h10a4 4 0 0 1 4 4v13H8a4 4 0 0 1-4-4z"/><path d="M18 21v-2"/></>),
  alert: (<><path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18.5v.01"/></>),
  close: (<><path d="M6 6l12 12M18 6L6 18"/></>),
};

window.Icon = Icon;
window.ICONS = ICONS;
