import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { audit, type AuditEntry } from "../../api/observability";
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

  const list = useQuery({
    queryKey: ["audit", "search", submitted],
    queryFn: () =>
      audit.search({
        limit: 200,
        actor: submitted || undefined,
      }),
    refetchInterval: 30_000,
  });

  const rows: AuditEntry[] = list.data ?? [];

  const exportHref = useMemo(() => {
    const u = new URLSearchParams();
    u.set("format", "csv");
    if (submitted) u.set("actor", submitted);
    u.set("limit", "1000");
    return `${env.apiBase}/api/v1/audit/export?${u.toString()}`;
  }, [submitted]);

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <input
            className="input"
            placeholder="Search audit log…"
            style={{ width: 240 }}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") setSubmitted(search);
            }}
          />
          <a className="btn btn--sm" href={exportHref} target="_blank" rel="noreferrer">
            Export CSV
          </a>
        </div>
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
            {rows.map((a, i) => (
              <tr key={a.id ?? i}>
                <td className="muted">{fmtAt(a.at)}</td>
                <td>{a.actor ?? "—"}</td>
                <td className="mono" style={{ fontSize: 11 }}>{a.action ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>{a.resource ?? "—"}</td>
                <td>
                  {a.result === "ok" || a.result === "success" ? (
                    <span className="pill pill--ok"><span className="dot" />ok</span>
                  ) : (
                    <span className="pill pill--err"><span className="dot" />{a.result ?? "fail"}</span>
                  )}
                </td>
                <td className="mono muted" style={{ fontSize: 11 }}>{a.ip ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.isLoading && <div className="muted" style={{ padding: 8 }}>Loading audit log…</div>}
        {list.isError && (
          <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
            Failed to load: {(list.error as Error).message}
          </div>
        )}
        {list.data && rows.length === 0 && (
          <div className="muted" style={{ padding: 20 }}>No audit entries.</div>
        )}
      </div>
    </div>
  );
}
