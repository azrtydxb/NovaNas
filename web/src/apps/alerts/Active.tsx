import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { alerts, type Alert } from "../../api/observability";
import { Icon } from "../../components/Icon";

function severityOf(a: Alert): string {
  return a.labels?.severity ?? "info";
}

function pillClass(sev: string): string {
  if (sev === "critical") return "pill pill--err";
  if (sev === "warning") return "pill pill--warn";
  if (sev === "info") return "pill pill--info";
  return "pill";
}

function fmtSince(ts?: string): string {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  const diff = Date.now() - d.getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function alertName(a: Alert): string {
  return a.labels?.alertname ?? a.fingerprint.slice(0, 8);
}

export default function Active() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["alerts", "list"],
    queryFn: () => alerts.list(),
    refetchInterval: 15000,
  });
  const list: Alert[] = q.data ?? [];
  const [sel, setSel] = useState<string | null>(null);
  const cur = list.find((a) => a.fingerprint === sel) ?? list[0];

  const counts = {
    critical: list.filter((a) => severityOf(a) === "critical").length,
    warning: list.filter((a) => severityOf(a) === "warning").length,
    info: list.filter((a) => severityOf(a) === "info").length,
  };

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 320px", height: "100%" }}>
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="pill pill--err">
            <span className="dot" /> {counts.critical} critical
          </span>
          <span className="pill pill--warn">
            <span className="dot" /> {counts.warning} warning
          </span>
          <span className="pill pill--info">
            <span className="dot" /> {counts.info} info
          </span>
          <button
            className="btn btn--sm"
            style={{ marginLeft: "auto" }}
            onClick={() => qc.invalidateQueries({ queryKey: ["alerts", "list"] })}
          >
            <Icon name="refresh" size={11} /> Refresh
          </button>
        </div>

        {q.isLoading && <div className="muted">Loading alerts…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            No active alerts.
          </div>
        )}

        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Alert</th>
                <th>Severity</th>
                <th>Since</th>
                <th>Labels</th>
              </tr>
            </thead>
            <tbody>
              {list.map((a) => {
                const sev = severityOf(a);
                const isOn = (cur && cur.fingerprint === a.fingerprint) || false;
                return (
                  <tr
                    key={a.fingerprint}
                    className={isOn ? "is-on" : ""}
                    onClick={() => setSel(a.fingerprint)}
                  >
                    <td>{alertName(a)}</td>
                    <td>
                      <span className={pillClass(sev)}>
                        <span className="dot" /> {sev}
                      </span>
                    </td>
                    <td className="muted">{fmtSince(a.startsAt)}</td>
                    <td className="mono muted" style={{ fontSize: 10 }}>
                      {Object.entries(a.labels ?? {})
                        .filter(([k]) => k !== "alertname" && k !== "severity")
                        .slice(0, 4)
                        .map(([k, v]) => `${k}=${v}`)
                        .join(" ")}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div>
              <div className="muted mono" style={{ fontSize: 10 }}>
                ALERT · {cur.fingerprint.slice(0, 12)}
              </div>
              <div className="side-detail__title">{alertName(cur)}</div>
            </div>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">Summary</div>
            </div>
            <div className="sect__body" style={{ fontSize: 12 }}>
              {cur.annotations?.summary ??
                cur.annotations?.description ??
                "No summary."}
            </div>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">Labels</div>
            </div>
            <table className="tbl tbl--compact">
              <tbody>
                {Object.entries(cur.labels ?? {}).map(([k, v]) => (
                  <tr key={k}>
                    <td className="mono">{k}</td>
                    <td className="mono">{v}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">State</div>
            </div>
            <dl className="kv">
              <dt>Severity</dt>
              <dd>{severityOf(cur)}</dd>
              <dt>State</dt>
              <dd>{cur.status?.state ?? "—"}</dd>
              <dt>Since</dt>
              <dd>{cur.startsAt ?? "—"}</dd>
            </dl>
          </div>

          <div
            className="row gap-8"
            style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
          >
            <button className="btn btn--sm">Silence…</button>
            {cur.generatorURL && (
              <a
                className="btn btn--sm"
                href={cur.generatorURL}
                target="_blank"
                rel="noreferrer"
              >
                Source
              </a>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
