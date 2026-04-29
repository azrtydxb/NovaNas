/* globals React */
const { useState, useEffect, useRef, useCallback } = React;

// ───────── Window manager ─────────
// Each window: { id, app, title, x, y, w, h, z, min, max }

function useWindowManager(initial = []) {
  const [wins, setWins] = useState(initial);
  const zRef = useRef(initial.reduce((m, w) => Math.max(m, w.z || 0), 0));

  const open = useCallback((spec) => {
    setWins(prev => {
      const existing = prev.find(w => w.id === spec.id);
      if (existing) {
        zRef.current += 1;
        return prev.map(w => w.id === spec.id ? { ...w, min: false, z: zRef.current } : w);
      }
      zRef.current += 1;
      return [...prev, { x: 60, y: 60, w: 720, h: 480, min: false, max: false, ...spec, z: zRef.current }];
    });
  }, []);

  const close = useCallback((id) => setWins(prev => prev.filter(w => w.id !== id)), []);
  const focus = useCallback((id) => {
    setWins(prev => {
      const w = prev.find(x => x.id === id);
      if (!w || w.z === zRef.current) return prev;
      zRef.current += 1;
      return prev.map(x => x.id === id ? { ...x, min: false, z: zRef.current } : x);
    });
  }, []);
  const toggleMin = useCallback((id) => setWins(prev => prev.map(w => w.id === id ? { ...w, min: !w.min } : w)), []);
  const toggleMax = useCallback((id) => {
    setWins(prev => prev.map(w => {
      if (w.id !== id) return w;
      if (w.max) return { ...w, max: false, x: w._x ?? w.x, y: w._y ?? w.y, ww: w._w ?? w.w, hh: w._h ?? w.h };
      return { ...w, max: true, _x: w.x, _y: w.y, _w: w.w, _h: w.h };
    }));
  }, []);
  const move = useCallback((id, x, y) => setWins(prev => prev.map(w => w.id === id ? { ...w, x, y } : w)), []);
  const resize = useCallback((id, w, h) => setWins(prev => prev.map(x => x.id === id ? { ...x, w, h } : x)), []);

  return { wins, open, close, focus, toggleMin, toggleMax, move, resize };
}

// Drag/resize hook for a window element
function useWindowDrag(win, mgr, surfaceRef) {
  const onTitleDown = useCallback((e) => {
    if (win.max) return;
    if (e.target.closest('.win__btns')) return;
    e.preventDefault();
    mgr.focus(win.id);
    const startX = e.clientX, startY = e.clientY;
    const ox = win.x, oy = win.y;
    const surf = surfaceRef.current;
    const onMove = (ev) => {
      let nx = ox + (ev.clientX - startX);
      let ny = oy + (ev.clientY - startY);
      if (surf) {
        const sw = surf.clientWidth, sh = surf.clientHeight;
        nx = Math.max(-win.w + 80, Math.min(sw - 80, nx));
        ny = Math.max(0, Math.min(sh - 40, ny));
      }
      mgr.move(win.id, nx, ny);
    };
    const onUp = () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }, [win, mgr, surfaceRef]);

  const onResizeDown = useCallback((e) => {
    if (win.max) return;
    e.preventDefault(); e.stopPropagation();
    mgr.focus(win.id);
    const startX = e.clientX, startY = e.clientY;
    const ow = win.w, oh = win.h;
    const onMove = (ev) => {
      const nw = Math.max(360, ow + (ev.clientX - startX));
      const nh = Math.max(240, oh + (ev.clientY - startY));
      mgr.resize(win.id, nw, nh);
    };
    const onUp = () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }, [win, mgr]);

  return { onTitleDown, onResizeDown };
}

// ───────── Sparkline ─────────
function Spark({ data, w = 200, h = 36, color = "currentColor", filled = true }) {
  if (!data?.length) return null;
  const lo = Math.min(...data), hi = Math.max(...data);
  const range = hi - lo || 1;
  const step = w / (data.length - 1);
  const pts = data.map((v, i) => [i * step, h - ((v - lo) / range) * h]);
  const d = pts.map((p, i) => (i ? `L${p[0]},${p[1]}` : `M${p[0]},${p[1]}`)).join(" ");
  const uid = "sp" + Math.random().toString(36).slice(2, 7);
  return (
    <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{ width: "100%", height: h, display: "block" }}>
      {filled && (<>
        <defs>
          <linearGradient id={uid} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.32"/>
            <stop offset="100%" stopColor={color} stopOpacity="0"/>
          </linearGradient>
        </defs>
        <path d={`${d} L${w},${h} L0,${h} Z`} fill={`url(#${uid})`} stroke="none"/>
      </>)}
      <path d={d} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke"/>
    </svg>
  );
}

function useSeries(len = 48, base = 0.5, vol = 0.08, seed = 1) {
  const [s] = useState(() => Array.from({ length: len }, (_, i) => {
    const x = Math.sin((seed * 9301 + i * 49297) % 233280) * 43758.5453;
    const r = x - Math.floor(x);
    return Math.max(0, Math.min(1, base + (r - 0.5) * vol * 2));
  }));
  return s;
}

function Ring({ value = 0.6, size = 120, stroke = 10, color = "var(--accent)", track = "var(--bg-3)", label, sub }) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  return (
    <div style={{ position: "relative", width: size, height: size, display: "grid", placeItems: "center" }}>
      <svg width={size} height={size} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size/2} cy={size/2} r={r} strokeWidth={stroke} stroke={track} fill="none"/>
        <circle cx={size/2} cy={size/2} r={r} strokeWidth={stroke} stroke={color} fill="none"
          strokeDasharray={c} strokeDashoffset={c * (1 - value)} strokeLinecap="round"
          style={{ transition: "stroke-dashoffset 600ms ease" }}/>
      </svg>
      <div style={{ position: "absolute", textAlign: "center" }}>
        <div className="ring__label">{label}</div>
        {sub && <div className="ring__sub">{sub}</div>}
      </div>
    </div>
  );
}

window.useWindowManager = useWindowManager;
window.useWindowDrag = useWindowDrag;
window.Spark = Spark;
window.useSeries = useSeries;
window.Ring = Ring;
