// Right-rail desktop widgets matching the design screenshot:
//   1. System Health (capacity ring + 2x2 KPIs + "Action needed" badge)
//   2. Storage Pools (per-pool name + tier + bar + %)
//   3. Recent Activity (severity dot + line + timestamp)
//   4. Active Jobs (name + bar + duration)
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { storage, type Pool } from "../api/storage";
import { useWM } from "../wm/store";

export function Widgets() {
  return (
    <div className="widgets">
      <SystemHealthCard />
      <StoragePoolsCard />
      <RecentActivityCard />
      <ActiveJobsCard />
    </div>
  );
}

// ---------- System Health ---------------------------------------------------

type SystemInfo = {
  hostname?: string;
  uptime?: string | number; // backend returns Go duration string e.g. "100h8m9.95s"
  loadAvg?: number[];
};

type Disk = {
  name: string;
  state?: string;
  health?: string;
};

type AlertItem = {
  fingerprint: string;
  status?: { state?: string };
  state?: string;
};

function SystemHealthCard() {
  const open = useWM((s) => s.open);
  const pools = useQuery({
    queryKey: ["widget", "pools"],
    queryFn: () => storage.listPools(),
    refetchInterval: 30_000,
  });
  const disks = useQuery({
    queryKey: ["widget", "disks"],
    queryFn: () => api<Disk[]>("/api/v1/disks"),
    refetchInterval: 60_000,
  });
  const sys = useQuery({
    queryKey: ["widget", "system"],
    queryFn: () => api<SystemInfo>("/api/v1/system/info"),
    refetchInterval: 30_000,
  });
  const alerts = useQuery({
    queryKey: ["widget", "alerts"],
    queryFn: () => api<AlertItem[]>("/api/v1/alerts"),
    refetchInterval: 30_000,
  });

  const poolList = arr<Pool>(pools.data);
  const poolCount = poolList.length;
  const totalCap = poolList.reduce((m, p) => m + (p.total ?? p.size ?? 0), 0);
  const usedCap = poolList.reduce((m, p) => m + (p.used ?? p.alloc ?? 0), 0);
  const pct = totalCap > 0 ? usedCap / totalCap : 0;

  const diskList = arr<Disk>(disks.data);
  const totalDisks = diskList.length;
  const healthyDisks = diskList.filter((d) => {
    const s = (d.state ?? d.health ?? "").toLowerCase();
    return /online|healthy|ok|good|active/.test(s) || s === "";
  }).length;

  const firingAlerts = arr<AlertItem>(alerts.data).filter((a) => {
    const s = (a.status?.state ?? a.state ?? "").toLowerCase();
    return s === "" || s === "active" || s === "firing";
  }).length;

  const actionNeeded = firingAlerts > 0 || healthyDisks < totalDisks;

  // ring math: 96px diameter -> r=42, c=2πr
  const r = 42;
  const c = 2 * Math.PI * r;

  return (
    <div className="hwidget">
      <div className="hwidget__head">
        <span className="hwidget__title">System Health</span>
        {actionNeeded ? (
          <button
            className="hwidget__pill hwidget__pill--err"
            onClick={() => open("alerts")}
            title="Open Alerts"
          >
            <span className="dot" /> Action needed
          </button>
        ) : (
          <span className="hwidget__pill hwidget__pill--ok">
            <span className="dot" /> Healthy
          </span>
        )}
      </div>
      <div className="hwidget__body">
        <div className="ring2">
          <svg width="120" height="120" viewBox="0 0 120 120">
            <circle cx="60" cy="60" r={r} fill="none" stroke="var(--bg-3)" strokeWidth="8" />
            <circle
              cx="60"
              cy="60"
              r={r}
              fill="none"
              stroke="var(--accent)"
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={c}
              strokeDashoffset={c - c * pct}
              transform="rotate(-90 60 60)"
              style={{ transition: "stroke-dashoffset 300ms ease" }}
            />
          </svg>
          <div className="ring2__center">
            <div className="ring2__pct">{Math.round(pct * 100)}%</div>
            <div className="ring2__lbl">Capacity</div>
          </div>
        </div>
        <div className="hkpis">
          <Kpi label="Pools" value={String(poolCount)} onClick={() => open("storage")} />
          <Kpi
            label="Disks"
            value={`${healthyDisks}/${totalDisks || "—"}`}
            onClick={() => open("storage")}
          />
          <Kpi label="Uptime" value={fmtUptime(sys.data?.uptime)} />
          <Kpi
            label="Alerts"
            value={String(firingAlerts)}
            danger={firingAlerts > 0}
            onClick={() => open("alerts")}
          />
        </div>
      </div>
    </div>
  );
}

function Kpi({
  label,
  value,
  danger,
  onClick,
}: {
  label: string;
  value: string;
  danger?: boolean;
  onClick?: () => void;
}) {
  const Tag = onClick ? "button" : "div";
  return (
    <Tag className={`hkpi${onClick ? " hkpi--clickable" : ""}`} onClick={onClick}>
      <span className="hkpi__lbl">{label}</span>
      <span className={`hkpi__val${danger ? " hkpi__val--err" : ""}`}>{value}</span>
    </Tag>
  );
}

// ---------- Storage Pools --------------------------------------------------

function StoragePoolsCard() {
  const open = useWM((s) => s.open);
  const q = useQuery({
    queryKey: ["widget", "pools"],
    queryFn: () => storage.listPools(),
    refetchInterval: 30_000,
  });
  const pools = arr<Pool>(q.data);
  if (pools.length === 0) return null;
  return (
    <div className="hwidget">
      <div className="hwidget__head">
        <span className="hwidget__title">Storage Pools</span>
        <button className="hwidget__btn" onClick={() => open("storage")}>
          Manage
        </button>
      </div>
      <div className="hwidget__list">
        {pools.map((p) => (
          <PoolRow key={p.name} p={p} />
        ))}
      </div>
    </div>
  );
}

function PoolRow({ p }: { p: Pool }) {
  const used = p.used ?? p.alloc ?? 0;
  const total = p.total ?? p.size ?? 0;
  const pct = total > 0 ? used / total : 0;
  const tier = (p.tier ?? "warm").toLowerCase();
  const dotColor =
    tier === "hot" ? "var(--err)" : tier === "warm" ? "var(--warn)" : "var(--info)";
  const protection = p.protection ?? "";
  return (
    <div className="hpool">
      <span className="hpool__dot" style={{ background: dotColor }} />
      <span className="hpool__name">{p.name}</span>
      {protection && <span className="hpool__type mono">{protection}</span>}
      <span className="hpool__bar">
        <span className="hpool__bar-fill" style={{ width: `${pct * 100}%` }} />
      </span>
      <span className="hpool__pct mono">{Math.round(pct * 100)}%</span>
    </div>
  );
}

// ---------- Recent Activity ------------------------------------------------

type AuditItem = {
  id?: number | string;
  timestamp?: string; // backend's actual field
  at?: string;
  ts?: string;
  actor?: string;
  action?: string;
  resource?: string;
  result?: string;
  outcome?: string; // backend's actual field: "success" | "rejected" | "error"
  message?: string;
};

function RecentActivityCard() {
  const open = useWM((s) => s.open);
  const q = useQuery({
    queryKey: ["widget", "audit"],
    queryFn: () => api<AuditItem[]>("/api/v1/audit?limit=5"),
    refetchInterval: 20_000,
  });
  const items = arr<AuditItem>(q.data);
  return (
    <div className="hwidget">
      <div className="hwidget__head">
        <span className="hwidget__title">Recent Activity</span>
        <button className="hwidget__btn" onClick={() => open("audit")}>
          All
        </button>
      </div>
      <div className="hwidget__list">
        {q.isLoading && <div className="hwidget__msg muted">Loading…</div>}
        {q.isError && <div className="hwidget__msg muted">No activity feed available</div>}
        {q.data && items.length === 0 && (
          <div className="hwidget__msg muted">No recent activity.</div>
        )}
        {items.map((it, i) => (
          <ActivityRow key={it.id ?? i} item={it} />
        ))}
      </div>
    </div>
  );
}

function ActivityRow({ item }: { item: AuditItem }) {
  const ts = item.timestamp ?? item.at ?? item.ts;
  const time = ts
    ? new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : "—";
  const outcome = (item.outcome ?? item.result ?? "success").toLowerCase();
  const dotColor =
    outcome === "success" || outcome === "ok"
      ? "var(--ok)"
      : outcome === "rejected" || outcome === "denied"
        ? "var(--warn)"
        : "var(--err)";
  const text = humanizeAction(item);
  return (
    <div className="hact">
      <span className="hact__dot" style={{ background: dotColor }} />
      <span className="hact__text">{text}</span>
      <span className="hact__ts mono">{time}</span>
    </div>
  );
}

// Turn an audit row into a human-readable line: "scrub pool tank" rather
// than the raw "POST /api/v1/pools/tank/scrub".
function humanizeAction(item: AuditItem): string {
  if (item.message) return item.message;
  const a = item.action ?? "";
  // POST /api/v1/foo/bar/action  →  "action foo/bar"
  const m = a.match(/^(\w+)\s+\/api\/v\d+\/(.+)$/);
  if (!m) return a || "—";
  const verb = m[1];
  const path = m[2];
  const segs = path.split("/");
  const last = segs[segs.length - 1];
  const looksLikeAction =
    /^[a-z][a-z-]+$/.test(last) &&
    !/[a-f0-9]{8,}/i.test(last) &&
    last.length < 24;
  if (looksLikeAction && segs.length >= 2) {
    const target = segs.slice(0, -1).join("/");
    return `${last} ${target}`;
  }
  if (verb === "DELETE") return `delete ${path}`;
  if (verb === "POST") return `create ${path}`;
  if (verb === "PUT" || verb === "PATCH") return `update ${path}`;
  return `${verb.toLowerCase()} ${path}`;
}

// ---------- Active Jobs ----------------------------------------------------

type Job = {
  id: string;
  kind?: string;
  state?: string;
  progress?: number;
  startedAt?: string;
  eta?: string;
  ts?: string;
};

function ActiveJobsCard() {
  const open = useWM((s) => s.open);
  const q = useQuery({
    queryKey: ["widget", "jobs"],
    queryFn: () => api<Job[]>("/api/v1/jobs"),
    refetchInterval: 5_000,
  });
  const jobs = arr<Job>(q.data).filter((j) => /running|active|in.?progress/i.test(j.state ?? ""));
  return (
    <div className="hwidget">
      <div className="hwidget__head">
        <span className="hwidget__title">Active Jobs</span>
        <button className="hwidget__btn" onClick={() => open("jobs")}>
          All
        </button>
      </div>
      <div className="hwidget__list">
        {jobs.length === 0 && <div className="hwidget__msg muted">No jobs running.</div>}
        {jobs.map((j) => {
          const elapsed = j.startedAt ? Date.now() - new Date(j.startedAt).getTime() : 0;
          return (
            <div key={j.id} className="hjob">
              <span className="hjob__name">{j.kind ?? j.id}</span>
              <span className="hjob__bar">
                <span
                  className="hjob__bar-fill"
                  style={{ width: `${Math.min(100, (j.progress ?? 0) * 100)}%` }}
                />
              </span>
              <span className="hjob__t mono">{fmtDuration(elapsed)}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------- helpers --------------------------------------------------------

// Coerce an arbitrary backend response into an array. Defends against
// `{items: [...]}`, `{plugins: [...]}`, null, error objects, etc.
function arr<T>(v: unknown): T[] {
  if (Array.isArray(v)) return v as T[];
  if (v && typeof v === "object") {
    for (const k of ["items", "data", "results", "rows"]) {
      const inner = (v as Record<string, unknown>)[k];
      if (Array.isArray(inner)) return inner as T[];
    }
  }
  return [];
}

// Backend emits uptime as a Go duration string ("100h8m9.95s") OR as
// a raw number of seconds. Normalize and display as `Nd Nh` / `Nh Nm`.
function fmtUptime(v?: string | number): string {
  if (v == null) return "—";
  const sec = typeof v === "number" ? v : parseGoDuration(v);
  if (!sec || sec < 60) return sec ? `${Math.round(sec)}s` : "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  if (d > 0) return `${d}d ${h}h`;
  const m = Math.floor((sec % 3600) / 60);
  return `${h}h ${m}m`;
}

function parseGoDuration(s: string): number {
  let total = 0;
  let m;
  const re = /([\d.]+)(h|m|s|ms|us|µs|ns)/g;
  while ((m = re.exec(s)) != null) {
    const n = parseFloat(m[1]);
    switch (m[2]) {
      case "h": total += n * 3600; break;
      case "m": total += n * 60; break;
      case "s": total += n; break;
      case "ms": total += n / 1000; break;
      default: break;
    }
  }
  return total;
}

function fmtDuration(ms: number): string {
  if (ms <= 0) return "—";
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  return `${hr}h ${min % 60}m`;
}
