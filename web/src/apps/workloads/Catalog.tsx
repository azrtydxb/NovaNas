import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { workloads, type ChartIndexEntry } from "../../api/workloads";

export function Catalog() {
  const [filter, setFilter] = useState("");
  const q = useQuery({
    queryKey: ["workloads", "index"],
    queryFn: () => workloads.index(),
    retry: false,
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 14 }} className="muted">
          Catalog index unavailable while k3s initializes.
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load catalog: {err.message}
      </div>
    );
  }

  const entries: ChartIndexEntry[] = q.data ?? [];
  const filtered = filter
    ? entries.filter((a) =>
        (a.displayName ?? a.name).toLowerCase().includes(filter.toLowerCase()),
      )
    : entries;

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <input
          className="input"
          placeholder="Search catalog…"
          style={{ width: 240 }}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <span className="muted" style={{ fontSize: 11, marginLeft: "auto" }}>
          {entries.length} chart{entries.length === 1 ? "" : "s"}
        </span>
        <button className="btn btn--sm" onClick={() => q.refetch()}>
          Refresh
        </button>
      </div>
      {q.isLoading && <div className="muted">Loading catalog…</div>}
      {!q.isLoading && filtered.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No charts in the catalog.
        </div>
      )}
      {filtered.length > 0 && (
        <div className="appcards">
          {filtered.slice(0, 30).map((a) => (
            <div key={a.name} className="appcard">
              <div
                className="appcard__icon"
                style={{
                  background: a.color
                    ? `linear-gradient(135deg, ${a.color}, ${a.color})`
                    : "linear-gradient(135deg, var(--accent), var(--accent-2, var(--accent)))",
                }}
              >
                {(a.displayName ?? a.name).slice(0, 2).toUpperCase()}
              </div>
              <div className="appcard__name">{a.displayName ?? a.name}</div>
              <div className="appcard__cat muted">
                {(a.category ?? "chart")} · v{a.version ?? "—"}
              </div>
              <button className="btn btn--sm btn--primary" style={{ marginTop: "auto" }}>
                Install
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
