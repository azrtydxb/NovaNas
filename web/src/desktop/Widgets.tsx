import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { storage } from "../api/storage";
import { Icon } from "../components/Icon";

// Per design README "Desktop widgets": Storage capacity ring, Pool list,
// Activity feed, Resource monitor sparklines.
export function Widgets() {
  return (
    <div className="widgets">
      <CapacityRingWidget />
      <PoolsWidget />
      <ResourceMonitorWidget />
    </div>
  );
}

function CapacityRingWidget() {
  const q = useQuery({
    queryKey: ["pools", "for-ring"],
    queryFn: () => storage.listPools(),
    refetchInterval: 30_000,
  });
  const pools = q.data ?? [];
  const total = pools.reduce((m, p) => m + (p.total ?? p.size ?? 0), 0);
  const used = pools.reduce((m, p) => m + (p.used ?? p.alloc ?? 0), 0);
  const pct = total > 0 ? used / total : 0;

  const r = 38;
  const c = 2 * Math.PI * r;
  return (
    <div className="widget">
      <div className="widget__title">
        <Icon name="storage" size={11} /> Capacity
      </div>
      <div className="ring">
        <svg className="ring__svg" width="96" height="96" viewBox="0 0 96 96">
          <circle className="ring__bg" cx="48" cy="48" r={r} fill="none" strokeWidth="6" />
          <circle
            className="ring__fg"
            cx="48"
            cy="48"
            r={r}
            fill="none"
            strokeWidth="6"
            strokeLinecap="round"
            strokeDasharray={c}
            strokeDashoffset={c - c * pct}
          />
        </svg>
        <div className="ring__center">
          <div className="ring__pct">{(pct * 100).toFixed(0)}%</div>
          <div className="ring__lbl">used</div>
        </div>
      </div>
      <div className="ring__sub">
        {fmtShort(used)} / {fmtShort(total)} across {pools.length} pool
        {pools.length === 1 ? "" : "s"}
      </div>
    </div>
  );
}

function PoolsWidget() {
  const q = useQuery({
    queryKey: ["pools", "for-widget"],
    queryFn: () => storage.listPools(),
    refetchInterval: 30_000,
  });
  const pools = q.data ?? [];
  if (pools.length === 0) return null;
  return (
    <div className="widget">
      <div className="widget__title">
        <Icon name="storage" size={11} /> Pools
      </div>
      {pools.map((p) => {
        const used = p.used ?? p.alloc ?? 0;
        const total = p.total ?? p.size ?? 0;
        const pct = total > 0 ? used / total : 0;
        return (
          <div key={p.name} className="widget__row">
            <span className="widget__name">{p.name}</span>
            <span className="widget__bar">
              <span className="widget__bar-fill" style={{ width: `${pct * 100}%` }} />
            </span>
            <span className="widget__pct">{(pct * 100).toFixed(0)}%</span>
          </div>
        );
      })}
    </div>
  );
}

type SystemInfo = {
  cpuPercent?: number;
  memUsed?: number;
  memTotal?: number;
  loadAvg?: number[];
  uptime?: number;
  hostname?: string;
};

function ResourceMonitorWidget() {
  const q = useQuery<SystemInfo>({
    queryKey: ["system-info-widget"],
    queryFn: () => api<SystemInfo>("/api/v1/system/info"),
    refetchInterval: 5_000,
  });
  const cpuRef = useRef<number[]>([]);
  const memRef = useRef<number[]>([]);
  const [, force] = useState(0);

  useEffect(() => {
    if (!q.data) return;
    const cpuV = q.data.cpuPercent ?? 0;
    const memV = q.data.memTotal ? ((q.data.memUsed ?? 0) / q.data.memTotal) * 100 : 0;
    cpuRef.current = [...cpuRef.current.slice(-29), cpuV];
    memRef.current = [...memRef.current.slice(-29), memV];
    force((n) => n + 1);
  }, [q.data]);

  const cpuNow = cpuRef.current[cpuRef.current.length - 1] ?? 0;
  const memNow = memRef.current[memRef.current.length - 1] ?? 0;

  return (
    <div className="widget">
      <div className="widget__title">
        <Icon name="monitor" size={11} /> Resources
      </div>
      <div className="spark__row">
        <span className="spark__lbl">CPU</span>
        <span className="spark" style={{ flex: 1 }}>
          {cpuRef.current.map((v, i) => (
            <span key={i} className="spark__bar" style={{ height: `${Math.max(2, v)}%` }} />
          ))}
        </span>
        <span className="spark__val">{cpuNow.toFixed(0)}%</span>
      </div>
      <div className="spark__row">
        <span className="spark__lbl">Memory</span>
        <span className="spark" style={{ flex: 1 }}>
          {memRef.current.map((v, i) => (
            <span key={i} className="spark__bar" style={{ height: `${Math.max(2, v)}%` }} />
          ))}
        </span>
        <span className="spark__val">{memNow.toFixed(0)}%</span>
      </div>
      {q.data?.loadAvg && (
        <div className="widget__row">
          <span className="muted small">load</span>
          <span className="mono small" style={{ marginLeft: "auto" }}>
            {q.data.loadAvg.map((x) => x.toFixed(2)).join(" ")}
          </span>
        </div>
      )}
    </div>
  );
}

function fmtShort(n: number): string {
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)}K`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(0)}M`;
  if (n < 1024 ** 4) return `${(n / 1024 ** 3).toFixed(1)}G`;
  return `${(n / 1024 ** 4).toFixed(1)}T`;
}
