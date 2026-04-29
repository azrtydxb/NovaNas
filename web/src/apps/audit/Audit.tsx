import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { audit, type AuditEntry } from "../../api/observability";
import { Icon } from "../../components/Icon";
import { env } from "../../lib/env";

function fmtAt(at?: string): string {
  if (!at) return "—";
  const d = new Date(at);
  if (isNaN(d.getTime())) return at;
  return d.toLocaleString(undefined, { hour12: false });
}

export default function Audit() {
  const [search, setSearch] = useState("");
  const [submitted, setSubmitted] = useState("");

  const summary = useQuery({
    queryKey: ["audit", "summary"],
    queryFn: () => audit.summary(),
    staleTime: 60_000,
  });

  const list = useQuery({
    queryKey: ["audit", "search", submitted],
    queryFn: () => audit.search({ limit: 100, actor: submitted || undefined }),
    refetchInterval: 30_000,
  });

  const rows: AuditEntry[] = list.data ?? [];

  const filtered = submitted
    ? rows.filter((r) => {
        const s = submitted.toLowerCase();
        return (
          (r.actor ?? "").toLowerCase().includes(s) ||
          (r.action ?? "").toLowerCase().includes(s) ||
          (r.resource ?? "").toLowerCase().includes(s)
        );
      })
    : rows;

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div
          className="row gap-8"
          style={{ marginBottom: 12, flexWrap: "wrap" }}
        >
          <div className="kpi">
            <div className="kpi__lbl">Total</div>
            <div className="kpi__val mono">{summary.data?.total ?? "—"}</div>
          </div>
          <div className="kpi">
            <div className="kpi__lbl">Last 24h</div>
            <div className="kpi__val mono">{summary.data?.last24h ?? "—"}</div>
          </div>
          <div className="kpi">
            <div className="kpi__lbl">Failed</div>
            <div className="kpi__val mono">{summary.data?.failed ?? "—"}</div>
          </div>
          <div className="kpi">
            <div className="kpi__lbl">Actors</div>
            <div className="kpi__val mono">{summary.data?.actors ?? "—"}</div>
          </div>
        </div>

        <div className="tbar">
          <div className="appcenter-search" style={{ width: 280 }}>
            <Icon name="search" size={11} />
            <input
              placeholder="Search actor, action, resource…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") setSubmitted(search);
              }}
            />
          </div>
          <button className="btn btn--sm" onClick={() => setSubmitted(search)}>
            Search
          </button>
          <a
            className="btn btn--sm"
            href={`${env.apiBase}${audit.exportUrl("csv")}`}
            target="_blank"
            rel="noreferrer"
            style={{ marginLeft: "auto" }}
          >
            <Icon name="download" size={11} /> Export CSV
          </a>
        </div>

        {list.isLoading && <div className="muted">Loading audit log…</div>}
        {list.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(list.error as Error).message}
          </div>
        )}
        {list.data && filtered.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            No audit entries.
          </div>
        )}

        {filtered.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>When</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Resource</th>
                <th>Result</th>
                <th>IP</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((a, i) => (
                <tr key={a.id ?? i}>
                  <td className="muted">{fmtAt(a.at)}</td>
                  <td>{a.actor ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {a.action ?? "—"}
                  </td>
                  <td className="muted mono" style={{ fontSize: 11 }}>
                    {a.resource ?? "—"}
                  </td>
                  <td>
                    {a.result === "ok" || a.result === "success" ? (
                      <span className="pill pill--ok">
                        <span className="dot" /> ok
                      </span>
                    ) : (
                      <span className="pill pill--err">
                        <span className="dot" /> {a.result ?? "error"}
                      </span>
                    )}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {a.ip ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
