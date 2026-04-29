import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { jobs, type Job, type JobDetail } from "../../api/observability";
import { Icon } from "../../components/Icon";

function statePill(state?: string): string {
  if (state === "ok" || state === "completed" || state === "succeeded")
    return "pill pill--ok";
  if (state === "running" || state === "active") return "pill pill--info";
  if (state === "failed" || state === "error") return "pill pill--err";
  if (state === "scheduled" || state === "retry" || state === "queued" || state === "pending")
    return "pill pill--warn";
  return "pill";
}

function progressPct(j: Job): number {
  if (typeof j.progress === "number")
    return Math.max(0, Math.min(1, j.progress > 1 ? j.progress / 100 : j.progress));
  if (typeof j.pct === "number") {
    const v = j.pct > 1 ? j.pct / 100 : j.pct;
    return Math.max(0, Math.min(1, v));
  }
  return 0;
}

function isRunning(j: Job): boolean {
  return j.state === "running" || j.state === "active";
}
function isTerminal(j: Job): boolean {
  return (
    j.state === "ok" ||
    j.state === "completed" ||
    j.state === "succeeded" ||
    j.state === "failed" ||
    j.state === "error"
  );
}

function fmtAt(at?: string): string {
  if (!at) return "—";
  const d = new Date(at);
  if (isNaN(d.getTime())) return at;
  return d.toLocaleString(undefined, { hour12: false });
}

function lastLogLine(d?: JobDetail): string {
  if (!d) return "";
  if (d.logs && d.logs.length) return d.logs[d.logs.length - 1];
  if (d.log) {
    const parts = d.log.trim().split(/\r?\n/);
    return parts[parts.length - 1] ?? "";
  }
  if (d.error) return d.error;
  return "";
}

export default function Jobs() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);

  const q = useQuery({
    queryKey: ["jobs", "list"],
    queryFn: () => jobs.list(),
    refetchInterval: 5000,
  });
  const list: Job[] = q.data ?? [];

  const detail = useQuery({
    queryKey: ["jobs", "detail", sel],
    queryFn: () => jobs.get(sel as string),
    enabled: !!sel,
    refetchInterval: sel ? 5000 : false,
  });

  const retry = useMutation({
    mutationFn: (id: string) => jobs.retry(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["jobs"] }),
  });
  const cancel = useMutation({
    mutationFn: (id: string) => jobs.cancel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["jobs"] }),
  });

  const counts = {
    running: list.filter(isRunning).length,
    queued: list.filter(
      (j) => j.state === "queued" || j.state === "pending" || j.state === "scheduled"
    ).length,
    ok: list.filter(
      (j) => j.state === "ok" || j.state === "completed" || j.state === "succeeded"
    ).length,
    failed: list.filter((j) => j.state === "failed" || j.state === "error").length,
  };

  const cur = sel ? list.find((j) => j.id === sel) : null;
  const det: JobDetail | undefined = detail.data;
  const detailLogs: string[] = det?.logs
    ? det.logs
    : det?.log
      ? det.log.split(/\r?\n/)
      : [];

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: cur ? "1fr 360px" : "1fr",
        height: "100%",
      }}
    >
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="pill pill--info">
            <span className="dot" /> {counts.running} running
          </span>
          <span className="pill pill--warn">
            <span className="dot" /> {counts.queued} queued
          </span>
          <span className="pill pill--ok">
            <span className="dot" /> {counts.ok} done
          </span>
          {counts.failed > 0 && (
            <span className="pill pill--err">
              <span className="dot" /> {counts.failed} failed
            </span>
          )}
          <button
            className="btn btn--sm"
            style={{ marginLeft: "auto" }}
            onClick={() => qc.invalidateQueries({ queryKey: ["jobs"] })}
          >
            <Icon name="refresh" size={11} /> Refresh
          </button>
        </div>

        {q.isLoading && <div className="muted">Loading jobs…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            No jobs.
          </div>
        )}

        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Job</th>
                <th>Kind</th>
                <th>Target</th>
                <th>Progress</th>
                <th>ETA</th>
                <th>Started</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {list.map((j) => {
                const pct = progressPct(j);
                const running = isRunning(j);
                const isOn = sel === j.id;
                return (
                  <tr
                    key={j.id}
                    className={isOn ? "is-on" : ""}
                    onClick={() => setSel(j.id)}
                    style={{ cursor: "pointer" }}
                  >
                    <td className="mono" style={{ fontSize: 11 }}>
                      {j.id.slice(0, 12)}
                    </td>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {j.kind ?? "—"}
                    </td>
                    <td className="muted">{j.target ?? "—"}</td>
                    <td>
                      {running || pct > 0 ? (
                        <div className="cap">
                          <div className="cap__bar">
                            <div style={{ width: `${pct * 100}%` }} />
                          </div>
                          <span className="mono" style={{ fontSize: 11 }}>
                            {Math.round(pct * 100)}%
                          </span>
                        </div>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {j.eta ?? "—"}
                    </td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {fmtAt(j.startedAt)}
                    </td>
                    <td>
                      <span className={statePill(j.state)}>
                        <span className="dot" /> {j.state ?? "unknown"}
                      </span>
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
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="muted mono" style={{ fontSize: 10 }}>
                JOB · {cur.id.slice(0, 12)}
              </div>
              <div className="side-detail__title">{cur.kind ?? "job"}</div>
            </div>
            <button
              className="modal__close"
              onClick={() => setSel(null)}
              title="Close"
            >
              <Icon name="x" size={14} />
            </button>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">State</div>
            </div>
            <dl className="kv">
              <dt>State</dt>
              <dd>
                <span className={statePill(cur.state)}>
                  <span className="dot" /> {cur.state ?? "unknown"}
                </span>
              </dd>
              <dt>Target</dt>
              <dd className="mono">{cur.target ?? "—"}</dd>
              <dt>Queue</dt>
              <dd className="mono">{cur.queue ?? "—"}</dd>
              <dt>Started</dt>
              <dd className="muted">{fmtAt(cur.startedAt)}</dd>
              <dt>Completed</dt>
              <dd className="muted">{fmtAt(cur.completedAt)}</dd>
              <dt>Retries</dt>
              <dd className="mono">
                {cur.retried ?? 0}
                {typeof cur.maxRetry === "number" ? ` / ${cur.maxRetry}` : ""}
              </dd>
              <dt>ETA</dt>
              <dd className="mono">{cur.eta ?? "—"}</dd>
              <dt>Progress</dt>
              <dd>
                <div className="cap">
                  <div className="cap__bar">
                    <div style={{ width: `${progressPct(cur) * 100}%` }} />
                  </div>
                  <span className="mono" style={{ fontSize: 11 }}>
                    {Math.round(progressPct(cur) * 100)}%
                  </span>
                </div>
              </dd>
            </dl>
          </div>

          {cur.error && (
            <div className="sect">
              <div className="sect__head">
                <div className="sect__title">Error</div>
              </div>
              <div
                className="sect__body mono"
                style={{ fontSize: 11, color: "var(--err)" }}
              >
                {cur.error}
              </div>
            </div>
          )}

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">Log</div>
            </div>
            <div
              className="log-stream"
              style={{ maxHeight: 240, padding: 6 }}
            >
              {detail.isLoading && <div className="muted">Loading…</div>}
              {detail.isError && (
                <div className="muted" style={{ color: "var(--err)" }}>
                  {(detail.error as Error).message}
                </div>
              )}
              {detailLogs.length === 0 && detail.data && (
                <div className="muted">No log lines.</div>
              )}
              {detailLogs.slice(-200).map((line, i) => (
                <div key={i} className="log-line">
                  <div></div>
                  <div></div>
                  <div></div>
                  <div className="log-line__msg">{line}</div>
                </div>
              ))}
            </div>
            {det && (
              <div
                className="muted mono"
                style={{ fontSize: 10, padding: "4px 12px" }}
              >
                last: {lastLogLine(det) || "—"}
              </div>
            )}
          </div>

          <div
            className="row gap-8"
            style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
          >
            <button
              className="btn btn--sm"
              disabled={!isTerminal(cur) || retry.isPending}
              onClick={() => retry.mutate(cur.id)}
              title="POST /jobs/{id}/retry — backend may not implement yet"
            >
              <Icon name="refresh" size={11} /> Retry
            </button>
            <button
              className="btn btn--sm btn--danger"
              disabled={isTerminal(cur) || cancel.isPending}
              onClick={() => {
                if (confirm(`Cancel job ${cur.id.slice(0, 8)}?`)) {
                  cancel.mutate(cur.id);
                }
              }}
              title="DELETE /jobs/{id} — backend may not implement yet"
            >
              <Icon name="stop" size={11} /> Cancel
            </button>
            {(retry.isError || cancel.isError) && (
              <span
                className="muted"
                style={{ fontSize: 10, color: "var(--warn)" }}
              >
                Action endpoint may not be implemented yet.
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
