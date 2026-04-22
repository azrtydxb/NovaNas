/* globals React */
const { useState: useStatePr, useEffect: useEffectPr, useRef: useRefPr, useMemo: useMemoPr } = React;

// ───────── fake time streams (for WS-style live metrics) ─────────
function useTicker(intervalMs = 1500) {
  const [t, setT] = useStatePr(0);
  useEffectPr(() => {
    const id = setInterval(() => setT(x => x + 1), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return t;
}

// deterministic pseudo-random from a seed+index, so sparkline shapes stay stable between renders
function rand(seed, i) {
  const x = Math.sin((seed * 9301 + i * 49297) % 233280) * 43758.5453;
  return x - Math.floor(x);
}

function useSparkSeries(length = 48, baseline = 0.5, volatility = 0.08, seed = 1, intervalMs = 1500) {
  const [series, setSeries] = useStatePr(() =>
    Array.from({ length }, (_, i) => baseline + (rand(seed, i) - 0.5) * volatility * 2)
  );
  useEffectPr(() => {
    const id = setInterval(() => {
      setSeries(prev => {
        const next = prev.slice(1);
        const last = prev[prev.length - 1];
        const target = baseline + (Math.random() - 0.5) * volatility * 4;
        next.push(Math.max(0, Math.min(1, last * 0.6 + target * 0.4)));
        return next;
      });
    }, intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, baseline, volatility]);
  return series;
}

// ───────── Sparkline SVG ─────────
function Sparkline({ data, width = 200, height = 40, color = "currentColor", filled = true, gridY = false, min, max }) {
  if (!data || data.length === 0) return null;
  const lo = min ?? Math.min(...data);
  const hi = max ?? Math.max(...data);
  const range = hi - lo || 1;
  const pad = 1;
  const w = width, h = height - pad * 2;
  const step = w / (data.length - 1);
  const pts = data.map((v, i) => {
    const x = i * step;
    const y = pad + h - ((v - lo) / range) * h;
    return [x, y];
  });
  const d = pts.map((p, i) => (i === 0 ? `M${p[0]},${p[1]}` : `L${p[0]},${p[1]}`)).join(" ");
  const dFill = `${d} L${w},${height} L0,${height} Z`;
  const uid = Math.random().toString(36).slice(2, 8);

  return (
    <svg className="spark" viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" style={{ width: "100%", height }}>
      {gridY && (
        <>
          <line x1="0" x2={w} y1={h * 0.25 + pad} y2={h * 0.25 + pad} stroke="currentColor" strokeOpacity="0.08" />
          <line x1="0" x2={w} y1={h * 0.5 + pad}  y2={h * 0.5 + pad}  stroke="currentColor" strokeOpacity="0.08" />
          <line x1="0" x2={w} y1={h * 0.75 + pad} y2={h * 0.75 + pad} stroke="currentColor" strokeOpacity="0.08" />
        </>
      )}
      {filled && (
        <>
          <defs>
            <linearGradient id={`g-${uid}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.32" />
              <stop offset="100%" stopColor={color} stopOpacity="0" />
            </linearGradient>
          </defs>
          <path d={dFill} fill={`url(#g-${uid})`} stroke="none" />
        </>
      )}
      <path d={d} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

// ───────── RingMeter: dashboard health ring ─────────
function RingMeter({ value = 0.64, label, sub, size = 168, stroke = 14, color = "var(--accent)" }) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const off = c * (1 - value);
  return (
    <div style={{ display: "grid", placeItems: "center", position: "relative", width: size, height: size }}>
      <svg width={size} height={size} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size/2} cy={size/2} r={r} strokeWidth={stroke} stroke="var(--bg-3)" fill="none" />
        <circle cx={size/2} cy={size/2} r={r} strokeWidth={stroke} stroke={color} fill="none"
          strokeDasharray={c} strokeDashoffset={off} strokeLinecap="round"
          style={{ transition: "stroke-dashoffset 600ms ease" }}
        />
      </svg>
      <div style={{ position: "absolute", textAlign: "center" }}>
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 28, color: "var(--fg-0)", letterSpacing: "-0.02em", fontWeight: 500 }}>
          {label}
        </div>
        {sub && <div style={{ fontSize: 11, color: "var(--fg-3)", marginTop: 2 }}>{sub}</div>}
      </div>
    </div>
  );
}

// ───────── Bytes/humanize ─────────
function fmtBytes(bytes, digits = 1) {
  if (bytes == null) return "—";
  const units = ["B","KB","MB","GB","TB","PB"];
  let v = bytes; let u = 0;
  while (v >= 1024 && u < units.length - 1) { v /= 1024; u++; }
  return `${v.toFixed(v >= 100 ? 0 : digits)} ${units[u]}`;
}
function fmtNum(n, digits = 1) {
  if (n >= 1e9) return (n/1e9).toFixed(digits) + "B";
  if (n >= 1e6) return (n/1e6).toFixed(digits) + "M";
  if (n >= 1e3) return (n/1e3).toFixed(digits) + "K";
  return Math.round(n).toString();
}
function fmtPct(v) { return `${Math.round(v*100)}%`; }

// ───────── CapacityBar ─────────
function CapacityBar({ used, total, thresh = { warn: 0.8, err: 0.92 }, showText = true }) {
  const pct = Math.min(1, used / total);
  const cls = pct >= thresh.err ? "cap__bar--err" : pct >= thresh.warn ? "cap__bar--warn" : "";
  return (
    <div className="cap">
      {showText && <div className="cap__num">{fmtBytes(used)}</div>}
      <div className={`cap__bar ${cls}`}><div style={{ width: `${pct * 100}%` }}/></div>
      {showText && <div className="cap__num">{fmtBytes(total)}</div>}
    </div>
  );
}

// ───────── Status dot ─────────
function StatusDot({ tone = "ok" }) {
  return <span className={`sdot sdot--${tone}`} />;
}

// ───────── Pill ─────────
function Pill({ tone, children, dot }) {
  const cls = tone ? `pill pill--${tone}` : "pill";
  return <span className={cls}>{dot && <span className="dot"/>}{children}</span>;
}

// ───────── KV list ─────────
function KV({ rows }) {
  return (
    <dl className="kv">
      {rows.map(([k, v]) => (<React.Fragment key={k}><dt>{k}</dt><dd>{v}</dd></React.Fragment>))}
    </dl>
  );
}

window.Sparkline = Sparkline;
window.RingMeter = RingMeter;
window.CapacityBar = CapacityBar;
window.StatusDot = StatusDot;
window.Pill = Pill;
window.KV = KV;
window.useTicker = useTicker;
window.useSparkSeries = useSparkSeries;
window.fmtBytes = fmtBytes;
window.fmtNum = fmtNum;
window.fmtPct = fmtPct;
