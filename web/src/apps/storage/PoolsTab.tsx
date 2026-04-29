import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Pool } from "../../api/storage";
import { formatBytes } from "../../lib/format";

type Props = { onPick: (name: string) => void };

function poolUsed(p: Pool): number {
  return p.used ?? p.alloc ?? 0;
}
function poolTotal(p: Pool): number {
  return p.total ?? p.size ?? 0;
}

export function PoolsTab({ onPick }: Props) {
  const q = useQuery({ queryKey: ["pools"], queryFn: () => storage.listPools() });
  const pools = q.data ?? [];
  const totalSize = pools.reduce((m, p) => m + poolTotal(p), 0);

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} />
          Create pool
        </button>
        <button className="btn">
          <Icon name="download" size={11} />
          Import
        </button>
        <span
          className="muted"
          style={{ marginLeft: "auto", fontSize: 11 }}
        >
          {pools.length} pools · {formatBytes(totalSize)} total
        </span>
      </div>
      {q.isLoading && <div className="empty-hint">Loading pools…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && pools.length === 0 && <div className="empty-hint">No pools.</div>}
      {pools.length > 0 && (
        <div className="cards-grid">
          {pools.map((p) => {
            const used = poolUsed(p);
            const total = poolTotal(p);
            const pct = total > 0 ? used / total : 0;
            const tier = p.tier ?? "warm";
            const state = p.state ?? p.health ?? "ONLINE";
            const healthy = /online|healthy/i.test(state);
            return (
              <div
                key={p.name}
                className="pool-card"
                onClick={() => onPick(p.name)}
                style={{ cursor: "pointer" }}
              >
                <div className="pool-card__head">
                  <div className="pool-card__name">
                    <Icon name="storage" size={14} />
                    <span>{p.name}</span>
                    <span className={`tier tier--${tier}`}>{tier}</span>
                  </div>
                  <span className={`pill pill--${healthy ? "ok" : "warn"}`}>
                    <span className="dot" />
                    {state}
                  </span>
                </div>
                <div className="pool-card__meta">
                  <span>
                    {p.disks ?? "—"} disks · {p.devices ?? "—"}
                  </span>
                  <span>{p.protection ?? "—"}</span>
                </div>
                <div className="bar">
                  <div style={{ width: `${pct * 100}%` }} />
                </div>
                <div className="pool-card__nums">
                  <span className="mono">
                    {formatBytes(used)} / {formatBytes(total)}
                  </span>
                  <span className="muted mono">{(pct * 100).toFixed(1)}%</span>
                </div>
                <div className="pool-card__io">
                  <div>
                    <span className="muted">R</span>{" "}
                    <span className="mono">{p.throughput?.r ?? 0} MB/s</span>
                  </div>
                  <div>
                    <span className="muted">W</span>{" "}
                    <span className="mono">{p.throughput?.w ?? 0} MB/s</span>
                  </div>
                  <div>
                    <span className="muted">IOPS</span>{" "}
                    <span className="mono">
                      {((p.iops?.r ?? 0) / 1000).toFixed(1)}k
                    </span>
                  </div>
                </div>
                <div className="pool-card__scrub">
                  <span className="muted">scrub: {p.scrubLast ?? "—"}</span>
                  <span className="muted">next: {p.scrubNext ?? "—"}</span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export default PoolsTab;
