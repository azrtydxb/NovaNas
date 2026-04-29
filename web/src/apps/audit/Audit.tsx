import { useMemo, useState } from "react";
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

type Filters = {
  actor: string;
  action: string;
  since: string;
  limit: number;
};

export default function Audit() {
  const [draft, setDraft] = useState<Filters>({
    actor: "",
    action: "",
    since: "",
    limit: 100,
  });
  const [applied, setApplied] = useState<Filters>(draft);

  const summary = useQuery({
    queryKey: ["audit", "summary"],
    queryFn: () => audit.summary(),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });

  const list = useQuery({
    queryKey: ["audit", "search", applied],
    queryFn: () =>
      audit.search({
        limit: applied.limit,
        actor: applied.actor || undefined,
        action: applied.action || undefined,
        since: applied.since
          ? new Date(applied.since).toISOString()
          : undefined,
      }),
    refetchInterval: 30_000,
  });

  const rows: AuditEntry[] = list.data ?? [];

  const exportHref = useMemo(() => {
    const u = new URLSearchParams();
    u.set("format", "csv");
    if (applied.actor) u.set("actor", applied.actor);
    if (applied.action) u.set("action", applied.action);
    if (applied.since) u.set("since", new Date(applied.since).toISOString());
    u.set("limit", String(applied.limit));
    return `${env.apiBase}/api/v1/audit/export?${u.toString()}`;
  }, [applied]);

  const topAction = summary.data?.topActions?.[0];

  const apply = () => setApplied(draft);
  const reset = () => {
    const blank: Filters = { actor: "", action: "", since: "", limit: 100 };
    setDraft(blank);
    setApplied(blank);
  };

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
            <div className="kpi__lbl">Unique actors</div>
            <div className="kpi__val mono">{summary.data?.actors ?? "—"}</div>
          </div>
          {topAction && (
            <div className="kpi">
              <div className="kpi__lbl">Top action</div>
              <div className="kpi__val mono" style={{ fontSize: 12 }}>
                {topAction.action}{" "}
                <span className="muted">×{topAction.count}</span>
              </div>
            </div>
          )}
        </div>

        <div className="tbar" style={{ flexWrap: "wrap", gap: 6 }}>
          <div className="appcenter-search" style={{ width: 180 }}>
            <Icon name="user" size={11} />
            <input
              placeholder="Actor…"
              value={draft.actor}
              onChange={(e) => setDraft({ ...draft, actor: e.target.value })}
              onKeyDown={(e) => {
                if (e.key === "Enter") apply();
              }}
            />
          </div>
          <div className="appcenter-search" style={{ width: 180 }}>
            <Icon name="bolt" size={11} />
            <input
              placeholder="Action…"
              value={draft.action}
              onChange={(e) => setDraft({ ...draft, action: e.target.value })}
              onKeyDown={(e) => {
                if (e.key === "Enter") apply();
              }}
            />
          </div>
          <input
            className="input"
            type="datetime-local"
            value={draft.since}
            onChange={(e) => setDraft({ ...draft, since: e.target.value })}
            style={{ width: 200 }}
            title="Since"
          />
          <input
            className="input"
            type="number"
            min={1}
            max={1000}
            value={draft.limit}
            onChange={(e) =>
              setDraft({ ...draft, limit: Number(e.target.value) || 100 })
            }
            style={{ width: 80 }}
            title="Limit"
          />
          <button className="btn btn--sm btn--primary" onClick={apply}>
            <Icon name="search" size={11} /> Search
          </button>
          <button className="btn btn--sm" onClick={reset}>
            Reset
          </button>
          <a
            className="btn btn--sm"
            href={exportHref}
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
        {list.data && rows.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            No audit entries.
          </div>
        )}

        {rows.length > 0 && (
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
                        <span className="dot" /> {a.result ?? "fail"}
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
